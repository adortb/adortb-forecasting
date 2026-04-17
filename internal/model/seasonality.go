package model

import "math"

// SeasonalityComponent 季节性分量（Fourier 级数）
type SeasonalityComponent struct {
	Period float64   // 周期（天）
	K      int       // Fourier 阶数
	Coeffs []float64 // [a1,b1,a2,b2,...,aK,bK]
}

// At 计算时间 t（天）处的季节性值
func (s *SeasonalityComponent) At(t float64) float64 {
	var sum float64
	for j := 1; j <= s.K; j++ {
		angle := 2 * math.Pi * float64(j) * t / s.Period
		idx := (j - 1) * 2
		sum += s.Coeffs[idx]*math.Cos(angle) + s.Coeffs[idx+1]*math.Sin(angle)
	}
	return sum
}

// FourierMatrix 为时间序列构建 Fourier 特征矩阵（每行 2K 列）
func FourierMatrix(ts []float64, period float64, K int) [][]float64 {
	mat := make([][]float64, len(ts))
	for i, t := range ts {
		row := make([]float64, 2*K)
		for j := 1; j <= K; j++ {
			angle := 2 * math.Pi * float64(j) * t / period
			row[(j-1)*2] = math.Cos(angle)
			row[(j-1)*2+1] = math.Sin(angle)
		}
		mat[i] = row
	}
	return mat
}

// FitSeasonality 拟合单个季节性分量
// ts: 天数序列, residuals: 残差序列（已去除趋势）
func FitSeasonality(ts, residuals []float64, period float64, K int) *SeasonalityComponent {
	X := FourierMatrix(ts, period, K)
	// 普通最小二乘 (X^T X)^-1 X^T r
	coeffs := leastSquares(X, residuals)
	return &SeasonalityComponent{Period: period, K: K, Coeffs: coeffs}
}

// leastSquares 求解最小二乘问题 min ||Xβ - y||^2，返回系数 β
func leastSquares(X [][]float64, y []float64) []float64 {
	if len(X) == 0 {
		return nil
	}
	nCols := len(X[0])
	// 构建正规方程 X^T X β = X^T y
	XtX := make([][]float64, nCols)
	Xty := make([]float64, nCols)
	for i := range XtX {
		XtX[i] = make([]float64, nCols)
	}
	for i, row := range X {
		for j, xij := range row {
			Xty[j] += xij * y[i]
			for l, xil := range row {
				XtX[j][l] += xij * xil
			}
		}
	}
	// 添加微小正则化防止奇异
	for j := 0; j < nCols; j++ {
		XtX[j][j] += 1e-6
	}
	return solveCholesky(XtX, Xty)
}

// Seasonality 组合多个季节性分量
type Seasonality struct {
	Components []*SeasonalityComponent
}

// At 计算时间 t（天）处的总季节性值
func (s *Seasonality) At(t float64) float64 {
	var total float64
	for _, c := range s.Components {
		total += c.At(t)
	}
	return total
}

// FitSeasonalities 同时拟合周季节性 (K=3) 和年季节性 (K=10)
func FitSeasonalities(ts, residuals []float64) *Seasonality {
	weekly := FitSeasonality(ts, residuals, 7.0, 3)
	yearly := FitSeasonality(ts, residuals, 365.25, 10)
	return &Seasonality{Components: []*SeasonalityComponent{weekly, yearly}}
}
