package ratelimit

import (
	"context"
	"fmt"
	"time"

	redis "github.com/sahithakellacodes/distributed-rate-limiter/internal/redis"
)

type RedisTokenBucketStrategy struct {
	client *redis.Client
	now    func() time.Time
}

func NewRedisTokenBucketStrategy(client *redis.Client) *RedisTokenBucketStrategy {
	return &RedisTokenBucketStrategy{
		client: client,
		now:    time.Now,
	}
}

func (s *RedisTokenBucketStrategy) Check(
	ctx context.Context,
	identifier string,
	config RateLimitConfig,
) (RateLimitResult, error) {
	redisKey := "ratelimit:tb:{" + identifier + "}"

	capacity := float64(config.MaxRequestsPerWindow)
	refillRate := capacity / float64(config.WindowSize.Nanoseconds())
	currentTime := s.now().UnixNano()

	var tokenBucketScript = redis.NewScript(`
		local capacity = tonumber(ARGV[1])
		local refillRate = tonumber(ARGV[2])
		local currentTime = tonumber(ARGV[3])

		local bucketExists = redis.call("EXISTS", KEYS[1])

		if bucketExists == 0 then
			redis.call("HSET", KEYS[1], "tokens", capacity, "lastRefill", currentTime)
		end

		local tokens = tonumber(redis.call("HGET", KEYS[1], "tokens"))
		local lastRefill = tonumber(redis.call("HGET", KEYS[1], "lastRefill"))

		local elapsed = currentTime - lastRefill

		if elapsed > 0 then
			tokens = math.min(tokens + elapsed * refillRate, capacity)
		end

		if tokens >= 1 then
			tokens = math.max(0, tokens - 1)
			redis.call("HSET", KEYS[1], "tokens", tokens, "lastRefill", currentTime)
			-- { allowed (1/0), remaining whole tokens, seconds until the next token (0 if allowed) }
			return {1, math.floor(tokens), 0}
		else
			local secondsUntilNextToken = (1 - tokens) / refillRate / 1e9
			redis.call("HSET", KEYS[1], "lastRefill", currentTime)
			-- { allowed (1/0), remaining whole tokens, seconds until the next token }
			return {0, math.floor(tokens), math.ceil(secondsUntilNextToken)}
		end
	`)

	// KEYS[1] = redisKey
	// ARGV[1] = capacity
	// ARGV[2] = refillRate
	// ARGV[3] = currentTime
	result, _ := s.client.RunScript(
		ctx,
		tokenBucketScript,
		[]string{redisKey},
		capacity,
		refillRate,
		currentTime,
	)

	values, ok := result.([]interface{})
	if !ok || len(values) != 3 {
		return RateLimitResult{
			Remaining: int(values[1].(int64)),
		}, fmt.Errorf("unexpected Redis script result")
	}

	return RateLimitResult{
		Allowed:           values[0].(int64) == 1,
		Remaining:         int(values[1].(int64)),
		RetryAfterSeconds: int(values[2].(int64)),
	}, nil
}
