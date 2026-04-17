package model

import (
	"math"
	"testing"
)

func TestLinearRegression(t *testing.T) {
	// y = 2x + 3
	xs := []float64{0, 1, 2, 3, 4}
	ys := []float64{3, 5, 7, 9, 11}
	k, m := linearRegression(xs, ys)
	if math.Abs(k-2.0) > 1e-6 {
		t.Errorf("slope: want 2.0, got %f", k)
	}
	if math.Abs(m-3.0) > 1e-6 {
		t.Errorf("intercept: want 3.0, got %f", m)
	}
}

func TestTrendAt(t *testing.T) {
	tr := &Trend{K: 1.0, M: 0.5, ChangePoints: []ChangePoint{
		{T: 0.5, Delta: 1.0},
	}}
	// t=0.3 < 0.5, only initial slope
	got := tr.At(0.3)
	want := 1.0*0.3 + 0.5
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("At(0.3): want %f, got %f", want, got)
	}

	// t=0.7 >= 0.5, slope becomes 2.0, intercept adjusts
	got = tr.At(0.7)
	// k=2, m = 0.5 - 1.0*0.5 = 0.0
	want = 2.0*0.7 + 0.0
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("At(0.7): want %f, got %f", want, got)
	}
}

func TestFitTrend_NoChangePoints(t *testing.T) {
	ts := make([]float64, 20)
	ys := make([]float64, 20)
	for i := range ts {
		ts[i] = float64(i) / 19.0
		ys[i] = 3.0*ts[i] + 1.0
	}
	tr := FitTrend(ts, ys, 0)
	if math.Abs(tr.K-3.0) > 0.01 {
		t.Errorf("slope: want ~3.0, got %f", tr.K)
	}
	if math.Abs(tr.M-1.0) > 0.01 {
		t.Errorf("intercept: want ~1.0, got %f", tr.M)
	}
}

func TestFitTrend_WithChangePoints(t *testing.T) {
	// 构造两段斜率不同的趋势
	n := 100
	ts := make([]float64, n)
	ys := make([]float64, n)
	for i := range ts {
		ts[i] = float64(i) / float64(n-1)
		if ts[i] < 0.5 {
			ys[i] = 2*ts[i] + 1
		} else {
			// 斜率变为 4
			ys[i] = 2*0.5 + 1 + 4*(ts[i]-0.5)
		}
	}
	tr := FitTrend(ts, ys, 5)
	// 检查两端点预测误差在合理范围内
	pred0 := tr.At(0)
	pred1 := tr.At(1)
	if math.Abs(pred0-1.0) > 0.5 {
		t.Errorf("At(0): want ~1.0, got %f", pred0)
	}
	if math.Abs(pred1-ys[n-1]) > 1.0 {
		t.Errorf("At(1): want ~%f, got %f", ys[n-1], pred1)
	}
}

func TestSolveCholesky(t *testing.T) {
	// 2x + y = 5, x + 3y = 7 => x=1.6, y=1.8
	A := [][]float64{{2, 1}, {1, 3}}
	b := []float64{5, 7}
	x := solveCholesky(A, b)
	if math.Abs(x[0]-1.6) > 1e-6 {
		t.Errorf("x[0]: want 1.6, got %f", x[0])
	}
	if math.Abs(x[1]-1.8) > 1e-6 {
		t.Errorf("x[1]: want 1.8, got %f", x[1])
	}
}
