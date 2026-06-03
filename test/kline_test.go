package test

import (
	"fmt"
	"time"
)

/**
1秒K线   → now / 1 * 1
5秒K线   → now / 5 * 5
10秒K线  → now / 10 * 10
1分钟K线 → now / 60 * 60
5分钟K线 → now / 300 * 300
1小时K线 → now / 3600 * 3600
4小时K线 → now / 14400 * 14400
1天K线   → now / 86400 * 86400
*/

// K线结构体（通用，1s/10s/1min都用这个）
type Kline struct {
	Open      float64 // 开盘价
	High      float64 // 最高价
	Low       float64 // 最低价
	Close     float64 // 收盘价
	Volume    float64 // 成交量
	Timestamp int64   // 窗口开始时间戳（秒）
}

// 聚合器：同时管理 1秒 和 10秒 K线
type KlineAggregator struct {
	// 1秒 K线
	current1s *Kline
	// 10秒 K线
	current10s *Kline
}

func NewKlineAggregator() *KlineAggregator {
	return &KlineAggregator{}
}

// 核心：添加一笔新交易（价格、成交量）
func (a *KlineAggregator) AddTick(price float64, volume float64) {
	now := time.Now().Unix() // 当前秒级时间

	// ========== 1. 处理 1秒 K线 ==========
	if a.current1s == nil || now != a.current1s.Timestamp {
		// 时间到了 → 闭合上一根，生成新K线
		if a.current1s != nil {
			fmt.Println("✅ 1秒K线闭合:", *a.current1s)
		}
		a.current1s = &Kline{Timestamp: now}
	}
	a.updateKline(a.current1s, price, volume)

	// ========== 2. 处理 10秒 K线 ==========
	tenSecTS := now / 10 * 10 // 向下取整到 10秒
	if a.current10s == nil || tenSecTS != a.current10s.Timestamp {
		if a.current10s != nil {
			fmt.Println("✅ 10秒K线闭合:", *a.current10s)
		}
		a.current10s = &Kline{Timestamp: tenSecTS}
	}
	a.updateKline(a.current10s, price, volume)
}

// 更新K线（通用方法：O/H/L/C/V 计算）
func (a *KlineAggregator) updateKline(k *Kline, price float64, volume float64) {
	if k.Open == 0 {
		k.Open = price // 第一笔 = 开盘
	}
	k.Close = price // 最新一笔 = 收盘
	k.Volume += volume

	// 更新最高
	if price > k.High {
		k.High = price
	}
	// 更新最低
	if k.Low == 0 || price < k.Low {
		k.Low = price
	}
}

func main() {
	agg := NewKlineAggregator()

	// 模拟：每 500ms 来一笔交易（1秒2笔，10秒20笔）
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	fmt.Println("开始实时聚合 1秒 + 10秒 K线...\n")
	for range ticker.C {
		// 模拟价格：100 ± 2 随机波动
		price := 100.0 + (float64(time.Now().UnixNano()%1000)/1000.0)*4 - 2
		volume := 1.0 // 模拟成交量

		agg.AddTick(price, volume)
	}
}
