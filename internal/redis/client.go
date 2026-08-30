package redis

import (
	"context"

	goredis "github.com/redis/go-redis/v9"
)

type Client struct {
	client *goredis.Client
}

func NewClient(addr string) *Client {
	return &Client{
		client: goredis.NewClient(&goredis.Options{
			Addr: addr,
		}),
	}
}

func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

type Script struct {
	script *goredis.Script
}

func NewScript(src string) *Script {
	return &Script{
		script: goredis.NewScript(src),
	}
}

func (c *Client) RunScript(
	ctx context.Context,
	script *Script,
	keys []string,
	args ...interface{},
) (interface{}, error) {
	return script.script.Run(ctx, c.client, keys, args...).Result()
}
