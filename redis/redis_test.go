package redisstore_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/bakhod1r/sessionx"
	redisstore "github.com/bakhod1r/sessionx/redis"
	"github.com/bakhod1r/sessionx/storetest"
)

func TestConformance(t *testing.T) {
	storetest.Run(t, "redis", func(t *testing.T) sessionx.Store {
		srv := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
		t.Cleanup(func() { _ = client.Close() })
		return redisstore.New(client, redisstore.Options{})
	}, storetest.Capabilities{Enumerates: true})
}
