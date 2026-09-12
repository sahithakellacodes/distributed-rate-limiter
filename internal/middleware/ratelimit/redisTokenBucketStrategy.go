package ratelimit

import (
	"context"
	"fmt"

	redis "github.com/sahithakellacodes/distributed-rate-limiter/internal/redis"
)

type RedisTokenBucketStrategy struct {
	client *redis.Client
}

func NewRedisTokenBucketStrategy(client *redis.Client) *RedisTokenBucketStrategy {
	return &RedisTokenBucketStrategy{
		client: client,
	}
}

var tokenBucketScript = redis.NewScript(`
	local capacity = tonumber(ARGV[1])
	local refillRate = tonumber(ARGV[2])

	-- Use Redis' own clock so all gateway instances share the same time source.
	-- TIME returns {seconds, microseconds}.
	local redisTime = redis.call("TIME")
	local currentTime =
		tonumber(redisTime[1]) * 1000000 +
		tonumber(redisTime[2])

	local bucketExists = redis.call("EXISTS", KEYS[1])

	if bucketExists == 0 then
		redis.call(
			"HSET",
			KEYS[1],
			"tokens",
			capacity,
			"lastRefill",
			currentTime
		)
	end

	local tokens = tonumber(redis.call("HGET", KEYS[1], "tokens"))
	local lastRefill = tonumber(redis.call("HGET", KEYS[1], "lastRefill"))

	local elapsed = currentTime - lastRefill

	if elapsed > 0 then
		tokens = math.min(
			tokens + elapsed * refillRate,
			capacity
		)
		lastRefill = currentTime
	end

	if tokens >= 1 then
		tokens = tokens - 1

		redis.call(
			"HSET",
			KEYS[1],
			"tokens",
			tokens,
			"lastRefill",
			lastRefill
		)

		return {
			1,
			math.floor(tokens),
			0
		}
	end

	local secondsUntilNextToken =
		(1 - tokens) / refillRate / 1000000

	redis.call(
		"HSET",
		KEYS[1],
		"tokens",
		tokens,
		"lastRefill",
		lastRefill
	)

	return {
		0,
		math.floor(tokens),
		math.ceil(secondsUntilNextToken)
	}
`)

func (s *RedisTokenBucketStrategy) Check(
	ctx context.Context,
	identifier string,
	config RateLimitConfig,
) (RateLimitResult, error) {
	redisKey := "ratelimit:tb:{" + identifier + "}"

	capacity := float64(config.MaxRequestsPerWindow)
	refillRate := capacity / float64(config.WindowSize.Microseconds())

	// KEYS[1] = redisKey
	// ARGV[1] = capacity
	// ARGV[2] = refillRate
	result, err := s.client.RunScript(
		ctx,
		tokenBucketScript,
		[]string{redisKey},
		capacity,
		refillRate,
	)

	if err != nil {
		return RateLimitResult{}, err
	}

	values, ok := result.([]interface{})
	if !ok || len(values) != 3 {
		return RateLimitResult{}, fmt.Errorf("unexpected Redis script result")
	}

	return RateLimitResult{
		Allowed:           values[0].(int64) == 1,
		Remaining:         int(values[1].(int64)),
		RetryAfterSeconds: int(values[2].(int64)),
	}, nil
}
