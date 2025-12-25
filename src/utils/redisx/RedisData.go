package redisx

import "time"

// RedisData 包裹缓存数据和逻辑过期时间
type RedisData[T any] struct {
	Data       T         `json:"data"`
	ExpireTime time.Time `json:"expireTime"`
}
