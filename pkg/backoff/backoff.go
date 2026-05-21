package backoff

import (
	"math/rand"
	"time"
)

// 指数退避的配置
type Config struct {
	// 初始退避（在第一次失败后等待多长时间再重试）
	BaseDelay time.Duration
	// 乘数（在重试失败后用于乘以退避的因子）需要大于1.
	Multiplier float64
	// 抖动（随机化退避的程度）
	Jitter float64
	// 最大退避时间（退避上限）
	MaxDelay time.Duration
}

// DefaultConfig 默认配置
var DefaultConfig = Config{
	BaseDelay:  1 * time.Second,
	Multiplier: 1.2,
	Jitter:     0.2,
	MaxDelay:   120 * time.Second,
}

// DefaultExpoential 默认指数退避
var DefaultExpoential = Exponential{Config: &DefaultConfig}

type Exponential struct {
	Config *Config
}

// Backoff 根据重试次数返回指数退避的等待时间
func (bc *Exponential) Backoff(retries int) time.Duration {
	// 起始大小，最大上限
	backoff, max := float64(bc.Config.BaseDelay), float64(bc.Config.MaxDelay)

	// 计算 第retries次 的回退数值
	for backoff < max && retries > 0 {
		backoff *= bc.Config.Multiplier
		retries--
	}

	// 如果得到的回退数字大于最大上限，则使用最大上限值
	if backoff > max {
		backoff = max
	}

	// 增加随机数
	backoff *= 1 + bc.Config.Jitter*(rand.Float64()*2-1)
	if backoff < 0 {
		return 0
	}
	return time.Duration(backoff)
}

// Backoff 根据重试次数返回指数退避的等待时间范围
func (bc *Exponential) Range(retries int) (time.Duration, time.Duration) {
	// 起始大小，最大上限
	backoff, max := float64(bc.Config.BaseDelay), float64(bc.Config.MaxDelay)

	// 计算 第retries次 的回退数值
	for backoff < max && retries > 0 {
		backoff *= bc.Config.Multiplier
		retries--
	}

	// 如果得到的回退数字大于最大上限，则使用最大上限值
	if backoff > max {
		backoff = max
	}

	// 获取范围
	minBackoff := backoff * (1 + bc.Config.Jitter*(-1))
	maxBackoff := backoff * (1 + bc.Config.Jitter*(1))
	if minBackoff < 0 {
		return 0, 0
	}
	return time.Duration(minBackoff), time.Duration(maxBackoff)
}
