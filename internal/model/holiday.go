package model

import "time"

// Holiday 节假日定义
type Holiday struct {
	Name       string
	Date       time.Time
	Multiplier float64 // 相对影响（1.0 = 无影响，1.2 = +20%，0.8 = -20%）
	WindowDays int     // 影响窗口（前后各 WindowDays 天）
}

// HolidayEffect 节假日效果加乘器
type HolidayEffect struct {
	holidays []Holiday
}

// NewDefaultHolidayEffect 返回预定义美国主要节假日
func NewDefaultHolidayEffect(year int) *HolidayEffect {
	holidays := []Holiday{
		{Name: "New Year", Date: time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC), Multiplier: 1.3, WindowDays: 2},
		{Name: "Valentine's Day", Date: time.Date(year, 2, 14, 0, 0, 0, 0, time.UTC), Multiplier: 1.1, WindowDays: 1},
		{Name: "Memorial Day", Date: memorialDay(year), Multiplier: 1.15, WindowDays: 2},
		{Name: "Independence Day", Date: time.Date(year, 7, 4, 0, 0, 0, 0, time.UTC), Multiplier: 1.2, WindowDays: 2},
		{Name: "Labor Day", Date: laborDay(year), Multiplier: 1.15, WindowDays: 2},
		{Name: "Halloween", Date: time.Date(year, 10, 31, 0, 0, 0, 0, time.UTC), Multiplier: 1.1, WindowDays: 1},
		{Name: "Thanksgiving", Date: thanksgiving(year), Multiplier: 1.25, WindowDays: 3},
		{Name: "Black Friday", Date: thanksgiving(year).Add(24 * time.Hour), Multiplier: 1.5, WindowDays: 2},
		{Name: "Cyber Monday", Date: thanksgiving(year).Add(4 * 24 * time.Hour), Multiplier: 1.4, WindowDays: 1},
		{Name: "Christmas", Date: time.Date(year, 12, 25, 0, 0, 0, 0, time.UTC), Multiplier: 1.35, WindowDays: 3},
		{Name: "New Year's Eve", Date: time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC), Multiplier: 1.2, WindowDays: 1},
	}
	return &HolidayEffect{holidays: holidays}
}

// EffectAt 返回给定日期的节假日加乘值（无节假日时返回 1.0）
func (h *HolidayEffect) EffectAt(d time.Time) float64 {
	result := 1.0
	for _, hol := range h.holidays {
		diff := absDays(d, hol.Date)
		if diff <= hol.WindowDays {
			// 距离越近影响越大（线性衰减）
			weight := 1.0 - float64(diff)/float64(hol.WindowDays+1)
			effect := 1.0 + (hol.Multiplier-1.0)*weight
			if effect > result {
				result = effect
			}
		}
	}
	return result
}

// AdditiveEffectAt 返回节假日的加法影响（乘数-1，用于加法分解）
func (h *HolidayEffect) AdditiveEffectAt(baseValue float64, d time.Time) float64 {
	return baseValue * (h.EffectAt(d) - 1.0)
}

func absDays(a, b time.Time) int {
	diff := a.Sub(b)
	days := int(diff.Hours() / 24)
	if days < 0 {
		return -days
	}
	return days
}

// memorialDay 返回5月最后一个星期一
func memorialDay(year int) time.Time {
	t := time.Date(year, 5, 31, 0, 0, 0, 0, time.UTC)
	for t.Weekday() != time.Monday {
		t = t.Add(-24 * time.Hour)
	}
	return t
}

// laborDay 返回9月第一个星期一
func laborDay(year int) time.Time {
	t := time.Date(year, 9, 1, 0, 0, 0, 0, time.UTC)
	for t.Weekday() != time.Monday {
		t = t.Add(24 * time.Hour)
	}
	return t
}

// thanksgiving 返回11月第四个星期四
func thanksgiving(year int) time.Time {
	t := time.Date(year, 11, 1, 0, 0, 0, 0, time.UTC)
	count := 0
	for {
		if t.Weekday() == time.Thursday {
			count++
			if count == 4 {
				return t
			}
		}
		t = t.Add(24 * time.Hour)
	}
}
