package model

import (
	"math"
	"testing"
	"time"
)

func TestFitModel_Basic(t *testing.T) {
	data := make([]TrainData, 60)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range data {
		data[i] = TrainData{
			Date:  base.Add(time.Duration(i) * 24 * time.Hour),
			Value: float64(1000 + i*5),
		}
	}
	fit, err := FitModel(data)
	if err != nil {
		t.Fatalf("FitModel error: %v", err)
	}
	if fit.Trend == nil {
		t.Fatal("Trend is nil")
	}
	if fit.Seasonality == nil {
		t.Fatal("Seasonality is nil")
	}
}

func TestFitModel_Empty(t *testing.T) {
	_, err := FitModel(nil)
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestComputeMAPE(t *testing.T) {
	actual := []float64{100, 200, 300}
	residuals := []float64{10, -20, 30}
	mape := computeMAPE(actual, residuals)
	// (10/100 + 20/200 + 30/300) / 3 = (0.1+0.1+0.1)/3 = 0.1
	if math.Abs(mape-0.1) > 1e-6 {
		t.Errorf("MAPE: want 0.1, got %f", mape)
	}
}

func TestNormalizeTime(t *testing.T) {
	ts := []float64{0, 5, 10}
	norm := normalizeTime(ts, 0, 10)
	want := []float64{0, 0.5, 1.0}
	for i, v := range norm {
		if math.Abs(v-want[i]) > 1e-9 {
			t.Errorf("norm[%d]: want %f, got %f", i, want[i], v)
		}
	}
}

func TestChangePointCount(t *testing.T) {
	cases := []struct {
		n    int
		want int
	}{
		{20, 3},
		{90, 10},
		{200, 25},
	}
	for _, c := range cases {
		got := changePointCount(c.n)
		if got != c.want {
			t.Errorf("changePointCount(%d): want %d, got %d", c.n, c.want, got)
		}
	}
}
