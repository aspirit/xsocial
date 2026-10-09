package utils

import (
	"time"
	_ "time/tzdata"
)

var TimeUtil = newTimeUtil("Asia/Shanghai")

type timeUtil struct {
	Zone     string
	Location *time.Location
}

func newTimeUtil(zone string) *timeUtil {
	location, _ := time.LoadLocation(zone)

	return &timeUtil{
		Zone:     zone,
		Location: location,
	}
}

func (*timeUtil) Now() int64 {
	return time.Now().UnixMilli()
}

// NowAdd returns Unix milliseconds for (now + d). Use for range queries/cleanup
// when storage uses millisecond timestamps (e.g. CreatedAt from TimeUtil.Now()).
func (*timeUtil) NowAdd(d time.Duration) int64 {
	return time.Now().Add(d).UnixMilli()
}

func (*timeUtil) Microtime() int64 {
	return time.Now().UnixMicro()
}

func (util *timeUtil) ToString(t time.Time) string {
	return t.In(util.Location).Format("2006-01-02 15:04:05")
}
