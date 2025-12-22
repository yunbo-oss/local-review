package config

import "os"

// GetEnv 获取环境变量，如果不存在则返回默认值
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
