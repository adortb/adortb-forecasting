package model

import (
	"math"
	"math/rand"
	"sort"
	"time"
)

// ForecastPoint 单天预测结果
type ForecastPoint struct {
	Date    time.Time
	Value   float64
	Lower80 float64
	Upper80 float64
	Lower95 float64
	Upper95 float64
}

// PredictConfig 预测配置
type PredictConfig struct {
	HorizonDays     int
	ConfidenceLevel float64 // 0.8 or 0.95
	BootstrapN      int     // bootstrap 采样次数
	RandSeed        int64
}

// DefaultPredictConfig 默认配置
func DefaultPredictConfig(horizon int) PredictConfig {
	return PredictConfig{
		HorizonDays:     horizon,
		ConfidenceLevel: 0.8,
		BootstrapN:      1000,
		RandSeed:        42,
	}
}

// Predict 生成未来 HorizonDays 天的预测
func Predict(fit *FitResult, startDate time.Time, cfg PredictConfig) []ForecastPoint {
	if cfg.BootstrapN <= 0 {
		cfg.BootstrapN = 1000
	}

	rng := rand.New(rand.NewSource(cfg.RandSeed))
	tRange := fit.TMax - fit.TMin
	if tRange < 1 {
		tRange = 1
	}

	points := make([]ForecastPoint, cfg.HorizonDays)

	for day := 0; day < cfg.HorizonDays; day++ {
		date := startDate.Add(time.Duration(day) * 24 * time.Hour)

		// t 相对于训练基准的天数
		tAbs := fit.TMin + tRange + float64(day+1)
		tNorm := tAbs / tRange
		tDays := tAbs

		// 点预测
		trendVal := fit.Trend.At(tNorm)
		seasonVal := fit.Seasonality.At(tDays)
		holidayMult := fit.Holiday.EffectAt(date)
		predicted := (trendVal + seasonVal) * holidayMult

		// Bootstrap 不确定性估计
		samples := bootstrapSamples(fit.Residuals, cfg.BootstrapN, rng)
		lower80, upper80 := quantile(samples, predicted, 0.10, 0.90)
		lower95, upper95 := quantile(samples, predicted, 0.025, 0.975)

		points[day] = ForecastPoint{
			Date:    date,
			Value:   math.Max(0, predicted),
			Lower80: math.Max(0, lower80),
			Upper80: math.Max(0, upper80),
			Lower95: math.Max(0, lower95),
			Upper95: math.Max(0, upper95),
		}
	}

	return points
}

// bootstrapSamples 从残差中 bootstrap 采样
func bootstrapSamples(residuals []float64, n int, rng *rand.Rand) []float64 {
	if len(residuals) == 0 {
		return make([]float64, n)
	}
	samples := make([]float64, n)
	for i := range samples {
		samples[i] = residuals[rng.Intn(len(residuals))]
	}
	return samples
}

// quantile 计算预测值加残差后的分位数区间
func quantile(residuals []float64, predicted, lowerQ, upperQ float64) (lower, upper float64) {
	vals := make([]float64, len(residuals))
	for i, r := range residuals {
		vals[i] = predicted + r
	}
	sort.Float64s(vals)
	n := len(vals)
	lower = vals[int(float64(n)*lowerQ)]
	upper = vals[int(math.Min(float64(n)*upperQ, float64(n-1)))]
	return
}

// ModelInfo 模型元信息
type ModelInfo struct {
	TrendSlope  float64 `json:"trend_slope"`
	MAPE        float64 `json:"mape"`
	Reliable    bool    `json:"reliable"`
	LowConf     bool    `json:"low_confidence"`
}

// GetModelInfo 提取模型摘要信息
func GetModelInfo(fit *FitResult) ModelInfo {
	slope := 0.0
	if fit.Trend != nil {
		slope = fit.Trend.K
	}
	return ModelInfo{
		TrendSlope: slope,
		MAPE:       fit.MAPE,
		Reliable:   fit.Reliable,
		LowConf:    fit.LowConf,
	}
}
