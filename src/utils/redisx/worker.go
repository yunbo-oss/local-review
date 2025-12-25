package redisx

import (
	"context"
	"local-review-go/src/config/redis"
	"time"
)

type RedisWorker struct{}

// RedisWork 是全局ID生成器实例
var RedisWork = &RedisWorker{}

const (
	BEGIN_TIMESTAMP int64 = 1704067201
	COUNT_BITS            = 32
)

func (*RedisWorker) NextId(keyPrefix string) (int64, error) {
	now := time.Now()
	nowSecond := now.UTC().Unix()
	timeStamp := nowSecond - BEGIN_TIMESTAMP
	format := now.Format("2006:01:02")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	count, err := redis.GetRedisClient().Incr(ctx, "icr:"+keyPrefix+":"+format).Result()
	if err != nil {
		return 0, err
	}

	return timeStamp<<COUNT_BITS | count, nil
}
