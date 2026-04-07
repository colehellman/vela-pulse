package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port        string
	DatabaseURL string
	RedisURL    string
	JWTSecret   string

	// Apple SIWA
	AppleClientID string // app bundle ID, e.g. "com.vela.pulse"

	// Redis stream / cache keys
	StreamName     string
	ConsumerGroup  string
	GlobalCacheKey string

	// Backpressure thresholds (Tier C scraper pause/resume)
	BackpressureHigh int
	BackpressureLow  int
}

func Load() (*Config, error) {
	c := &Config{
		Port:             getOrDefault("PORT", "8080"),
		DatabaseURL:      mustGet("DATABASE_URL"),
		RedisURL:         mustGet("REDIS_URL"),
		JWTSecret:        mustGet("JWT_SECRET"),
		AppleClientID:    getOrDefault("APPLE_CLIENT_ID", "com.vela.pulse"),
		StreamName:       getOrDefault("STREAM_NAME", "vela:articles"),
		ConsumerGroup:    getOrDefault("CONSUMER_GROUP", "gateway"),
		GlobalCacheKey:   getOrDefault("GLOBAL_CACHE_KEY", "vela:global:top200"),
		BackpressureHigh: 5000,
		BackpressureLow:  2000,
	}
	return c, nil
}

func mustGet(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required env var %s is not set", key))
	}
	return v
}

func getOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
