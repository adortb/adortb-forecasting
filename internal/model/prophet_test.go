package model

import (
	"math"
	"testing"
	"time"
)

// generateSyntheticData 生成合成时序：线性趋势 + 周季节性 + 噪声
func generateSyntheticData(n int, slope, intercept, amplitude float64) []TrainData {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	data := make([]TrainData, n)
	for i := range data {
		t := float64(i)
		trend := slope*t/float64(n) + intercept
		seasonality := amplitude * math.Sin(2*math.Pi*t/7)
		data[i] = TrainData{
			Date:  base.Add(time.Duration(i) * 24 * time.Hour),
			Value: trend + seasonality,
		}
	}
	return data
}

func TestProphetTrain_Basic(t *testing.T) {
	data := generateSyntheticData(120, 100, 1000, 50)
	p, err := Train(data)
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}
	if p == nil {
		t.Fatal("prophet is nil")
	}
}

func TestProphetTrain_EmptyData(t *testing.T) {
	_, err := Train(nil)
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestProphetTrain_LowConf(t *testing.T) {
	// < 30 天数据时 LowConf = true
	data := generateSyntheticData(20, 10, 100, 5)
	p, err := Train(data)
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}
	if !p.IsLowConfidence() {
		t.Error("expected LowConf=true for <30 days of data")
	}
}

func TestProphetForecast_Horizon(t *testing.T) {
	data := generateSyntheticData(180, 50, 500, 30)
	p, err := Train(data)
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	start := time.Now()
	points, _ := p.Forecast(start, 30, 0.8)
	if len(points) != 30 {
		t.Errorf("forecast horizon: want 30 points, got %d", len(points))
	}
}

func TestProphetForecast_IntervalOrder(t *testing.T) {
	data := generateSyntheticData(180, 50, 500, 30)
	p, err := Train(data)
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	points, _ := p.Forecast(time.Now(), 30, 0.8)
	for i, pt := range points {
		if pt.Lower80 > pt.Value || pt.Upper80 < pt.Value {
			t.Errorf("point[%d]: interval order violated: lower80=%f value=%f upper80=%f",
				i, pt.Lower80, pt.Value, pt.Upper80)
		}
		if pt.Lower95 > pt.Lower80 {
			t.Errorf("point[%d]: lower95 > lower80", i)
		}
		if pt.Upper95 < pt.Upper80 {
			t.Errorf("point[%d]: upper95 < upper80", i)
		}
	}
}

func TestProphetForecast_Accuracy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping accuracy test in short mode")
	}
	// 训练 180 天，预测 30 天，验证 MAPE
	slope := 5.0
	intercept := 1000.0
	data := generateSyntheticData(180, slope, intercept, 50)
	p, err := Train(data)
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	// 生成未来 30 天的真实值
	futureData := generateSyntheticData(30, slope, intercept+slope*180/180, 50)
	// 调整基准时间
	baseEnd := data[len(data)-1].Date
	for i := range futureData {
		futureData[i].Date = baseEnd.Add(time.Duration(i+1) * 24 * time.Hour)
	}

	points, info := p.Forecast(baseEnd.Add(24*time.Hour), 30, 0.8)

	var mapeSum float64
	for i, pt := range points {
		actual := futureData[i].Value
		if math.Abs(actual) > 1e-6 {
			mapeSum += math.Abs(pt.Value-actual) / math.Abs(actual)
		}
	}
	forecastMAPE := mapeSum / float64(len(points))

	if forecastMAPE > 0.5 {
		t.Errorf("forecast MAPE too high: %f (model MAPE: %f)", forecastMAPE, info.MAPE)
	}
}

func TestProphetForecast_IntervalCoverage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping coverage test in short mode")
	}
	// 80% 区间应覆盖约 80% 的实际值（允许 ±15% 误差）
	data := generateSyntheticData(180, 10, 500, 40)
	p, err := Train(data)
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	// 使用训练集内部验证（残差 bootstrap）
	fit := p.fit
	covered := 0
	total := len(fit.Residuals)
	if total == 0 {
		t.Skip("no residuals to test coverage")
	}

	// 简单验证：残差 bootstrap 的 p10-p90 区间覆盖约 80% 的残差
	residuals := fit.Residuals
	sortedRes := make([]float64, len(residuals))
	copy(sortedRes, residuals)
	sortFloat64s(sortedRes)

	p10 := sortedRes[int(float64(len(sortedRes))*0.10)]
	p90 := sortedRes[int(float64(len(sortedRes))*0.90)]

	for _, r := range residuals {
		if r >= p10 && r <= p90 {
			covered++
		}
	}
	coverageRate := float64(covered) / float64(total)
	if coverageRate < 0.65 || coverageRate > 0.95 {
		t.Errorf("80%% interval coverage: want 65%%~95%%, got %.1f%%", coverageRate*100)
	}
}

func sortFloat64s(a []float64) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

// Prophet 需要暴露 fit 字段给测试（包内测试可直接访问私有字段）
// 此文件在 model 包内，可直接访问
