package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

const maintenanceInterval = 1 * time.Hour

// RunPartmanMaintenance calls pg_partman's run_maintenance on a fixed interval.
// This creates new weekly partitions ahead of time and drops expired ones per
// the retention policy configured in migration 003.
//
// Without this, pg_partman only creates the initial set of partitions; articles
// written after the last pre-created partition will fail with a partition violation.
//
// Run in a goroutine: go db.RunPartmanMaintenance(ctx, pool, log)
func RunPartmanMaintenance(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger) {
	log.Info("partman maintenance started", zap.Duration("interval", maintenanceInterval))

	// Run once immediately at startup so we don't have to wait an hour.
	runOnce(ctx, pool, log)

	ticker := time.NewTicker(maintenanceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("partman maintenance stopped")
			return
		case <-ticker.C:
			runOnce(ctx, pool, log)
		}
	}
}

func runOnce(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger) {
	// analyze=true updates statistics on the new partitions immediately.
	_, err := pool.Exec(ctx, `SELECT partman.run_maintenance('public.articles', p_analyze := true)`)
	if err != nil {
		log.Error("partman maintenance failed", zap.Error(err))
		return
	}
	log.Debug("partman maintenance completed")
}
