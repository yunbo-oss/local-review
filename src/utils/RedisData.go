package utils

import "time"

type RedisData[T any] struct {
	ExpireTime time.Time `json:"expireTime"`
	Data       T         `json:"data"`
}
