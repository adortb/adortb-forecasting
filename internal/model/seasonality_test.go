package model

import (
	"math"
	"testing"
)

func TestFourierMatrix_Shape(t *testing.T) {
	ts := []float64{0, 7, 14}
	mat := FourierMatrix(ts, 7.0, 3)
	if len(mat) != 3 {
		t.Errorf("rows: want 3, got %d", len(mat))
	}
	if len(mat[0]) != 6 {
		t.Errorf("cols: want 6 (2*K), got %d", len(mat[0]))
	}
}

func TestSeasonalityFit_Weekly(t *testing.T) {
	// 纯周季节性: y = 2*cos(2π*t/7) + 3*sin(2π*t/7)
	n := 84 // 12 weeks
	ts := make([]float64, n)
	ys := make([]float64, n)
	for i := range ts {
		ts[i] = float64(i)
		ys[i] = 2*math.Cos(2*math.Pi*ts[i]/7) + 3*math.Sin(2*math.Pi*ts[i]/7)
	}
	comp := FitSeasonality(ts, ys, 7.0, 3)

	// 验证系数误差
	if math.Abs(comp.Coeffs[0]-2.0) > 0.1 {
		t.Errorf("a1: want ~2.0, got %f", comp.Coeffs[0])
	}
	if math.Abs(comp.Coeffs[1]-3.0) > 0.1 {
		t.Errorf("b1: want ~3.0, got %f", comp.Coeffs[1])
	}
}

func TestSeasonalityAt_Reconstruction(t *testing.T) {
	// 用已知系数，验证 At() 能还原
	comp := &SeasonalityComponent{
		Period: 7.0,
		K:      1,
		Coeffs: []float64{1.0, 0.0}, // cos 分量
	}
	// t=0: cos(0)=1
	if math.Abs(comp.At(0)-1.0) > 1e-9 {
		t.Errorf("At(0): want 1.0, got %f", comp.At(0))
	}
	// t=7: cos(2π)=1
	if math.Abs(comp.At(7)-1.0) > 1e-9 {
		t.Errorf("At(7): want 1.0, got %f", comp.At(7))
	}
}

func TestFitSeasonalities(t *testing.T) {
	// 纯周季节性残差
	n := 365
	ts := make([]float64, n)
	rs := make([]float64, n)
	for i := range ts {
		ts[i] = float64(i)
		rs[i] = math.Sin(2 * math.Pi * ts[i] / 7)
	}
	s := FitSeasonalities(ts, rs)
	if len(s.Components) != 2 {
		t.Errorf("components: want 2, got %d", len(s.Components))
	}
	// 在训练点上误差应该很小
	for i := 0; i < n; i += 7 {
		got := s.At(ts[i])
		want := rs[i]
		if math.Abs(got-want) > 0.5 {
			t.Errorf("At(%f): want %f, got %f", ts[i], want, got)
		}
	}
}
