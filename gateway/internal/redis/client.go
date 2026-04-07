package redis

import (
	"context"
	"fmt"
	"sync/atomic"

	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const KillSwitchChannel = "vela:killswitch"

type Client struct {
	rdb    *goredis.Client
	killed atomic.Bool
	log    *zap.Logger
}

func NewClient(url string, log *zap.Logger) (*Client, error) {
	opt, err := goredis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	c := &Client{
		rdb: goredis.NewClient(opt),
		log: log,
	}
	if err := c.rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return c, nil
}

// SubscribeKillSwitch listens on vela:killswitch for runtime logic toggles.
// Supported payloads: "kill:all", "resume:all".
func (c *Client) SubscribeKillSwitch(ctx context.Context) {
	sub := c.rdb.Subscribe(ctx, KillSwitchChannel)
	go func() {
		defer sub.Close()
		ch := sub.Channel()
		for msg := range ch {
			switch msg.Payload {
			case "kill:all":
				c.killed.Store(true)
				c.log.Warn("kill switch activated", zap.String("payload", msg.Payload))
			case "resume:all":
				c.killed.Store(false)
				c.log.Info("kill switch deactivated")
			default:
				c.log.Debug("unknown kill-switch payload", zap.String("payload", msg.Payload))
			}
		}
	}()
}

func (c *Client) IsKilled() bool       { return c.killed.Load() }
func (c *Client) Raw() *goredis.Client { return c.rdb }
