package util

import (
	"time"

	"github.com/jinzhu/now"
)

type Now struct {
	*now.Now
}

func New(t time.Time) *Now {
	return &Now{now.New(t)}
}

func (now *Now) BeginningOfNM(n int) time.Time {
	y, m, d := now.Date()
	h := now.Hour()
	mi := now.Minute()
	mi -= mi % n
	return time.Date(y, m, d, h, mi, 0, 0, now.Location())
}

func (now *Now) BeginningOfNH(n int) time.Time {
	y, m, d := now.Date()
	h := now.Hour()
	h -= h % n
	return time.Date(y, m, d, h, 0, 0, 0, now.Location())
}

func GetCandleTime(blockTime int64, intervalMinute int) (candleTime int64) {
	if intervalMinute < 60 {
		candleTime = New(time.Unix(blockTime, 0)).BeginningOfNM(intervalMinute).Unix()
	} else {
		candleTime = New(time.Unix(blockTime, 0)).BeginningOfNH(intervalMinute / 60).Unix()
	}
	return
}
