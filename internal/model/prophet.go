// Package model 实现 Prophet-like 时序分解预测模型
// y(t) = trend(t) + seasonality(t) + holiday(t) + noise
package model

import (
	"fmt"
	"time"
)

// Prophet 完整预测模型
type Prophet struct {
	fit *FitResult
}

// Train 训练模型
func Train(data []TrainData) (*Prophet, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("training data is empty")
	}
	fit, err := FitModel(data)
	if err != nil {
		return nil, fmt.Errorf("model fitting failed: %w", err)
	}
	return &Prophet{fit: fit}, nil
}

// Forecast 生成预测结果
func (p *Prophet) Forecast(startDate time.Time, horizonDays int, confidenceLevel float64) ([]ForecastPoint, ModelInfo) {
	cfg := DefaultPredictConfig(horizonDays)
	cfg.ConfidenceLevel = confidenceLevel
	points := Predict(p.fit, startDate, cfg)
	info := GetModelInfo(p.fit)
	return points, info
}

// IsReliable 模型是否可靠（MAPE < 30%，数据量足够）
func (p *Prophet) IsReliable() bool {
	return p.fit.Reliable
}

// IsLowConfidence 训练数据不足
func (p *Prophet) IsLowConfidence() bool {
	return p.fit.LowConf
}
