package main

import (
	"context"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/colehellman/vela-pulse/gateway/internal/config"
	"github.com/colehellman/vela-pulse/gateway/internal/db"
	"github.com/colehellman/vela-pulse/gateway/internal/feed"
	"github.com/colehellman/vela-pulse/gateway/internal/handler"
	"github.com/colehellman/vela-pulse/gateway/internal/middleware"
	redisc "github.com/colehellman/vela-pulse/gateway/internal/redis"
)

func main() {
	log, _ := zap.NewProduction()
	defer log.Sync() //nolint:errcheck

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("config", zap.Error(err))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("db pool", zap.Error(err))
	}
	defer pool.Close()

	rdb, err := redisc.NewClient(cfg.RedisURL, log)
	if err != nil {
		log.Fatal("redis", zap.Error(err))
	}
	rdb.SubscribeKillSwitch(ctx)

	// Run pg_partman maintenance hourly to create future weekly partitions.
	go db.RunPartmanMaintenance(ctx, pool, log)

	// Start the Redis Streams consumer in a background goroutine.
	consumer := feed.NewConsumer(rdb.Raw(), pool, feed.ConsumerConfig{
		StreamName:     cfg.StreamName,
		ConsumerGroup:  cfg.ConsumerGroup,
		GlobalCacheKey: cfg.GlobalCacheKey,
	}, log)
	go func() {
		if err := consumer.Run(ctx); err != nil {
			log.Error("consumer exited", zap.Error(err))
		}
	}()

	jwtSecret := []byte(cfg.JWTSecret)

	// Global middleware stack applied to all routes.
	ks := middleware.KillSwitch(rdb)
	optAuth := middleware.OptionalAuth(jwtSecret)

	chain := func(h http.Handler) http.Handler {
		return ks(optAuth(h))
	}

	mux := http.NewServeMux()
	mux.Handle("/health", &handler.HealthHandler{})
	mux.Handle("/v1/feed", chain(handler.NewFeedHandler(pool, rdb.Raw(), cfg.GlobalCacheKey, log)))
	mux.Handle("/v1/auth/siwa", ks(handler.NewSIWAHandler(
		pool,
		jwtSecret,
		cfg.AppleClientID,
		log,
	)))

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("server listening", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("listen", zap.Error(err))
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error("shutdown", zap.Error(err))
	}
}
