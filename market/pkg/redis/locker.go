package rds

import (
	"context"
	"errors"
	"fmt"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"time"
)

func Lock(ctx context.Context, kv *redis.Redis, key string, expire int) (lock *redis.RedisLock, ok bool, err error) {
	lock = redis.NewRedisLock(kv, key)
	lock.SetExpire(expire)
	ok, err = lock.AcquireCtx(ctx)
	if err != nil {
		err = fmt.Errorf("lock AcquireCtx err: %w", err)
		return
	}
	if !ok {
		err = fmt.Errorf("slot %v acquire lock failed", err)
		return
	}
	return
}

var ErrLockTimeout = errors.New("lock timeout")

func MustLock(ctx context.Context, kv *redis.Redis, key string, expire int, timeout int) (lock *redis.RedisLock, err error) {
	var ok bool
	timer := time.NewTimer(time.Duration(timeout) * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		case <-timer.C:
			err = ErrLockTimeout
			return
		default:
			lock, ok, err = Lock(ctx, kv, key, expire)
			if ok {
				return
			}
			time.Sleep(time.Millisecond * 100)
		}
	}
}

func ReleaseLock(lock *redis.RedisLock) {
	if lock != nil {
		_, err := lock.Release()
		if err != nil {
			logx.Errorf("release lock err: %v", err)
		}
	}
}
