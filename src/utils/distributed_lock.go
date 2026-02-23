package utils

import (
	"context"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

type DistributedLock struct {
	client    *redis.Client
	watchDogs map[string]context.CancelFunc // 存储看门狗的取消函数
	mutex     sync.Mutex                    // 保护watchDogs的并发访问
}

func NewDistributedLock(client *redis.Client) *DistributedLock {
	return &DistributedLock{
		client:    client,
		watchDogs: make(map[string]context.CancelFunc),
	}
}

// LockWithWatchDog 自动管理看门狗的加锁方法
func (dl *DistributedLock) LockWithWatchDog(ctx context.Context, key string, ttl time.Duration) (bool, string, error) {
	token := uuid.New().String()
	result, err := dl.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return false, "", err
	}

	if !result {
		return false, "", nil
	}

	// 启动看门狗
	dl.mutex.Lock()
	defer dl.mutex.Unlock()

	// 如果已有看门狗，先停止它（防止重复）
	if cancel, ok := dl.watchDogs[key]; ok {
		cancel()
	}

	// 创建新的看门狗上下文
	dogCtx, cancel := context.WithCancel(context.Background())
	dl.watchDogs[key] = cancel

	// 启动看门狗协程
	go dl.watchDog(dogCtx, key, token, ttl)

	return true, token, nil
}

// UnlockWithWatchDog 自动停止看门狗的解锁方法
func (dl *DistributedLock) UnlockWithWatchDog(ctx context.Context, key, token string) error {
	// 先停止看门狗
	dl.mutex.Lock()
	if cancel, ok := dl.watchDogs[key]; ok {
		cancel()
		delete(dl.watchDogs, key)
	}
	dl.mutex.Unlock()

	// 执行解锁操作
	script := `
        if redis.call("GET", KEYS[1]) == ARGV[1] then
            return redis.call("DEL", KEYS[1])
        else
            return 0
        end
    `
	_, err := dl.client.Eval(ctx, script, []string{key}, token).Result()
	return err
}

// watchDog 自动续期的看门狗实现
func (dl *DistributedLock) watchDog(ctx context.Context, key, token string, ttl time.Duration) {
	ticker := time.NewTicker(ttl / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 续期时验证令牌
			script := `
                if redis.call("GET", KEYS[1]) == ARGV[1] then
                    return redis.call("EXPIRE", KEYS[1], ARGV[2])
                else
                    return 0
                end
            `
			result, err := dl.client.Eval(ctx, script, []string{key}, token, int(ttl/time.Second)).Result()
			if err != nil || result == nil {
				logrus.Warnf("锁续期失败: key=%s, err=%v", key, err)
				return
			}

		case <-ctx.Done():
			return
		}
	}
}
