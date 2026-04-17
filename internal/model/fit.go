package model

import (
	"errors"
	"math"
	"time"
)

const (
	MinTrainDays     = 30
	MaxMAPEThreshold = 0.30
)

// TrainData 训练数据点
type TrainData struct {
	Date  time.Time
	Value float64
}

// FitResult 拟合结果
type FitResult struct {
	Trend      *Trend
	Seasonality *Seasonality
	Holiday    *HolidayEffect
	// 训练集残差（用于 bootstrap 不确定性估计）
	Residuals []float64
	// 时间归一化参数
	TMin float64
	TMax float64
	// MAPE 评估
	MAPE       float64
	Reliable   bool
	LowConf    bool // 数据不足
}

// FitModel 对给定时序数据拟合 Prophet-like 模型
func FitModel(data []TrainData) (*FitResult, error) {
	if len(data) == 0 {
		return nil, errors.New("empty training data")
	}

	result := &FitResult{LowConf: len(data) < MinTrainDays}

	// 转换为数值时间轴（天数）
	ts, ys := toTimeSeries(data)
	tMin, tMax := ts[0], ts[len(ts)-1]
	result.TMin = tMin
	result.TMax = tMax

	// 归一化时间到 [0,1]
	tsNorm := normalizeTime(ts, tMin, tMax)

	// 节假日效果
	year := data[0].Date.Year()
	holidayEffect := NewDefaultHolidayEffect(year)
	result.Holiday = holidayEffect

	// 提取节假日影响后的残差
	holidayAdj := make([]float64, len(data))
	for i, d := range data {
		holidayAdj[i] = ys[i] / holidayEffect.EffectAt(d.Date)
	}

	// 拟合趋势（使用归一化时间）
	nChangePoints := changePointCount(len(data))
	trend := FitTrend(tsNorm, holidayAdj, nChangePoints)
	result.Trend = trend

	// 计算趋势残差
	trendResiduals := make([]float64, len(ts))
	for i, t := range tsNorm {
		trendResiduals[i] = holidayAdj[i] - trend.At(t)
	}

	// 拟合季节性
	seasonality := FitSeasonalities(ts, trendResiduals)
	result.Seasonality = seasonality

	// 计算最终残差
	finalResiduals := make([]float64, len(ts))
	for i, t := range ts {
		predicted := trend.At(tsNorm[i]) + seasonality.At(t)
		finalResiduals[i] = ys[i] - predicted*holidayEffect.EffectAt(data[i].Date)
	}
	result.Residuals = finalResiduals

	// 计算 MAPE
	result.MAPE = computeMAPE(ys, finalResiduals)
	result.Reliable = result.MAPE <= MaxMAPEThreshold && !result.LowConf

	return result, nil
}

// changePointCount 根据数据量决定变更点数量
func changePointCount(n int) int {
	switch {
	case n < 60:
		return 3
	case n < 120:
		return 10
	default:
		return 25
	}
}

// toTimeSeries 将 TrainData 转为 (天数序列, 值序列)
func toTimeSeries(data []TrainData) (ts, ys []float64) {
	base := data[0].Date
	ts = make([]float64, len(data))
	ys = make([]float64, len(data))
	for i, d := range data {
		ts[i] = d.Date.Sub(base).Hours() / 24.0
		ys[i] = d.Value
	}
	return
}

// normalizeTime 将时间序列归一化到 [0,1]
func normalizeTime(ts []float64, tMin, tMax float64) []float64 {
	rng := tMax - tMin
	if rng < 1e-12 {
		rng = 1
	}
	norm := make([]float64, len(ts))
	for i, t := range ts {
		norm[i] = (t - tMin) / rng
	}
	return norm
}

// computeMAPE 计算平均绝对百分比误差
func computeMAPE(actual, residuals []float64) float64 {
	var sum float64
	var count int
	for i, y := range actual {
		if math.Abs(y) > 1e-10 {
			sum += math.Abs(residuals[i]) / math.Abs(y)
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}
