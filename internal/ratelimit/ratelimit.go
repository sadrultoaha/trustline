// Package ratelimit is a fixed-window limiter backed by Redis, with an
// in-memory fallback for tests.
package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter interface {
	// Allow increments the counter for key and reports whether it is within limit per window.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

type Redis struct{ C *redis.Client }

func (r *Redis) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	k := fmt.Sprintf("rl:%s:%d", key, time.Now().Unix()/int64(window.Seconds()))
	pipe := r.C.TxPipeline()
	incr := pipe.Incr(ctx, k)
	pipe.Expire(ctx, k, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return incr.Val() <= int64(limit), nil
}

type Memory struct {
	mu   sync.Mutex
	hits map[string]int
}

func NewMemory() *Memory { return &Memory{hits: map[string]int{}} }

func (m *Memory) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := fmt.Sprintf("%s:%d", key, time.Now().Unix()/int64(window.Seconds()))
	m.hits[k]++
	return m.hits[k] <= limit, nil
}

// Noop allows everything (used when Redis is unavailable in dev).
type Noop struct{}

func (Noop) Allow(context.Context, string, int, time.Duration) (bool, error) { return true, nil }
