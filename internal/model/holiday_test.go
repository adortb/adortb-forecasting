package model

import (
	"testing"
	"time"
)

func TestHolidayEffect_NoHoliday(t *testing.T) {
	h := NewDefaultHolidayEffect(2026)
	// 随机工作日，不在节假日附近
	d := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC) // Sunday, mid-March
	got := h.EffectAt(d)
	if got != 1.0 {
		t.Errorf("expected 1.0 for non-holiday, got %f", got)
	}
}

func TestHolidayEffect_Christmas(t *testing.T) {
	h := NewDefaultHolidayEffect(2026)
	christmas := time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)
	got := h.EffectAt(christmas)
	if got <= 1.0 {
		t.Errorf("expected multiplier > 1.0 on Christmas, got %f", got)
	}
}

func TestHolidayEffect_WindowDecay(t *testing.T) {
	h := NewDefaultHolidayEffect(2026)
	// Black Friday 在感恩节后一天，窗口为2天
	thanksgiving := thanksgiving(2026)
	bf := thanksgiving.Add(24 * time.Hour) // Black Friday 本身

	effectBF := h.EffectAt(bf)
	effectDayBefore := h.EffectAt(bf.Add(-24 * time.Hour)) // 感恩节（也在窗口内）

	// Black Friday 应该有效果
	if effectBF <= 1.0 {
		t.Errorf("expected effect > 1.0 on Black Friday, got %f", effectBF)
	}
	_ = effectDayBefore
}

func TestAbsDays(t *testing.T) {
	a := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	if absDays(a, b) != 4 {
		t.Errorf("absDays: want 4, got %d", absDays(a, b))
	}
	if absDays(b, a) != 4 {
		t.Errorf("absDays reversed: want 4, got %d", absDays(b, a))
	}
}

func TestLabourDay(t *testing.T) {
	ld := laborDay(2026)
	if ld.Weekday() != time.Monday {
		t.Errorf("Labor Day should be Monday, got %s", ld.Weekday())
	}
	if ld.Month() != time.September {
		t.Errorf("Labor Day should be in September, got %s", ld.Month())
	}
}

func TestThanksgiving(t *testing.T) {
	tg := thanksgiving(2026)
	if tg.Weekday() != time.Thursday {
		t.Errorf("Thanksgiving should be Thursday, got %s", tg.Weekday())
	}
	if tg.Month() != time.November {
		t.Errorf("Thanksgiving should be in November, got %s", tg.Month())
	}
}
