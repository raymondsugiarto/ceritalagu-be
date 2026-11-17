package cli

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/yourorg/music-api/config"
)

// redisChecker reports Redis (the job queue) reachability for /readyz.
type redisChecker struct {
	client *redis.Client
}

func newRedisChecker(c config.RedisConfig) *redisChecker {
	return &redisChecker{client: redis.NewClient(&redis.Options{Addr: c.Addr, Password: c.Password, DB: c.DB})}
}

func (r *redisChecker) Name() string { return "redis" }

func (r *redisChecker) Check(ctx context.Context) error {
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}
	return nil
}
