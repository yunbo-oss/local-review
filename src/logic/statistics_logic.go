package logic

import (
	"context"
	"fmt"
	"local-review-go/src/config/redis"
	"local-review-go/src/utils/redisx"
	"time"
)

type StatisticsLogic interface {
	QueryUV(ctx context.Context, date string) (int64, error)
	QueryCurrentUV(ctx context.Context) (int64, error)
}

type statisticsLogic struct{}

func NewStatisticsLogic() StatisticsLogic {
	return &statisticsLogic{}
}

func (l *statisticsLogic) QueryUV(ctx context.Context, date string) (int64, error) {
	key := redisx.UVKeyPrefix + date
	count, err := redis.GetRedisClient().PFCount(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("pfcount key=%s: %w", key, err)
	}
	return count, nil
}

func (l *statisticsLogic) QueryCurrentUV(ctx context.Context) (int64, error) {
	date := time.Now().Format("20060102")
	return l.QueryUV(ctx, date)
}
