package model

import "math"

// ChangePoint 分段线性趋势的变更点
type ChangePoint struct {
	T     float64
	Delta float64 // 斜率变化量
}

// Trend 分段线性趋势分量
type Trend struct {
	K           float64       // 初始斜率
	M           float64       // 初始截距
	ChangePoints []ChangePoint
}

// NewTrend 创建趋势分量，changePointTimes 为归一化后的时间点 [0,1]
func NewTrend(k, m float64, cps []ChangePoint) *Trend {
	return &Trend{K: k, M: m, ChangePoints: cps}
}

// At 计算时间 t（归一化）处的趋势值
func (tr *Trend) At(t float64) float64 {
	k := tr.K
	m := tr.M
	for _, cp := range tr.ChangePoints {
		if t >= cp.T {
			k += cp.Delta
			m -= cp.Delta * cp.T
		}
	}
	return k*t + m
}

// FitTrend 使用最小二乘法拟合分段线性趋势
// ts: 归一化时间序列, ys: 对应值（已去除季节性）
// nChangePoints: 变更点数量
func FitTrend(ts, ys []float64, nChangePoints int) *Trend {
	if len(ts) == 0 {
		return &Trend{}
	}

	// 先做整体线性回归获得初始斜率和截距
	k, m := linearRegression(ts, ys)

	if nChangePoints <= 0 || len(ts) < nChangePoints*2 {
		return &Trend{K: k, M: m}
	}

	// 均匀放置变更点（排除头尾各10%）
	cpTimes := makeChangePointTimes(ts, nChangePoints)
	deltas := fitChangePointDeltas(ts, ys, k, m, cpTimes)

	cps := make([]ChangePoint, len(cpTimes))
	for i, t := range cpTimes {
		cps[i] = ChangePoint{T: t, Delta: deltas[i]}
	}

	return &Trend{K: k, M: m, ChangePoints: cps}
}

// linearRegression 简单最小二乘线性回归，返回 (斜率k, 截距m)
func linearRegression(xs, ys []float64) (k, m float64) {
	n := float64(len(xs))
	if n == 0 {
		return
	}
	var sumX, sumY, sumXY, sumX2 float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumX2 += xs[i] * xs[i]
	}
	denom := n*sumX2 - sumX*sumX
	if math.Abs(denom) < 1e-12 {
		return 0, sumY / n
	}
	k = (n*sumXY - sumX*sumY) / denom
	m = (sumY - k*sumX) / n
	return
}

// makeChangePointTimes 在 ts 范围内均匀生成变更点位置
func makeChangePointTimes(ts []float64, n int) []float64 {
	tMin := ts[int(float64(len(ts))*0.1)]
	tMax := ts[int(float64(len(ts))*0.9)]
	step := (tMax - tMin) / float64(n+1)
	cps := make([]float64, n)
	for i := range cps {
		cps[i] = tMin + step*float64(i+1)
	}
	return cps
}

// fitChangePointDeltas 通过带正则化的最小二乘估计各变更点的斜率变化
func fitChangePointDeltas(ts, ys []float64, k, m float64, cpTimes []float64) []float64 {
	// 构建残差（减去初始线性趋势）
	residuals := make([]float64, len(ts))
	for i, t := range ts {
		residuals[i] = ys[i] - (k*t + m)
	}

	// 对每个变更点，构建指示函数矩阵并用正则最小二乘求解
	nCP := len(cpTimes)
	// A[i][j] = max(0, ts[i] - cpTimes[j])
	A := make([][]float64, len(ts))
	for i, t := range ts {
		row := make([]float64, nCP)
		for j, cp := range cpTimes {
			if t >= cp {
				row[j] = t - cp
			}
		}
		A[i] = row
	}

	// 正规方程 (A^T A + λI) δ = A^T r, λ=0.05 作为 L2 正则化
	lambda := 0.05
	AtA := make([][]float64, nCP)
	Atr := make([]float64, nCP)
	for i := range AtA {
		AtA[i] = make([]float64, nCP)
	}
	for i := range ts {
		for j := 0; j < nCP; j++ {
			Atr[j] += A[i][j] * residuals[i]
			for l := 0; l < nCP; l++ {
				AtA[j][l] += A[i][j] * A[i][l]
			}
		}
	}
	for j := 0; j < nCP; j++ {
		AtA[j][j] += lambda
	}

	return solveCholesky(AtA, Atr)
}

// solveCholesky 用 Cholesky 分解求解对称正定线性方程组 Ax=b
func solveCholesky(A [][]float64, b []float64) []float64 {
	n := len(b)
	L := make([][]float64, n)
	for i := range L {
		L[i] = make([]float64, n)
	}

	for i := 0; i < n; i++ {
		for j := 0; j <= i; j++ {
			sum := A[i][j]
			for k := 0; k < j; k++ {
				sum -= L[i][k] * L[j][k]
			}
			if i == j {
				if sum <= 0 {
					sum = 1e-10
				}
				L[i][j] = math.Sqrt(sum)
			} else {
				if math.Abs(L[j][j]) < 1e-12 {
					L[i][j] = 0
				} else {
					L[i][j] = sum / L[j][j]
				}
			}
		}
	}

	// 前向替换 Ly=b
	y := make([]float64, n)
	for i := 0; i < n; i++ {
		sum := b[i]
		for j := 0; j < i; j++ {
			sum -= L[i][j] * y[j]
		}
		if math.Abs(L[i][i]) < 1e-12 {
			y[i] = 0
		} else {
			y[i] = sum / L[i][i]
		}
	}

	// 后向替换 L^T x=y
	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		sum := y[i]
		for j := i + 1; j < n; j++ {
			sum -= L[j][i] * x[j]
		}
		if math.Abs(L[i][i]) < 1e-12 {
			x[i] = 0
		} else {
			x[i] = sum / L[i][i]
		}
	}
	return x
}
