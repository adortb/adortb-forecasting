// Package aggregator 按多维度组合分别训练和管理模型
package aggregator

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/adortb/adortb-forecasting/internal/fetch"
	"github.com/adortb/adortb-forecasting/internal/model"
)

// DimensionKey 维度组合键
type DimensionKey struct {
	SlotID     string
	Country    string
	DeviceType string
}

// String 返回可作为缓存键的字符串
func (d DimensionKey) String() string {
	return fmt.Sprintf("%s|%s|%s", d.SlotID, d.Country, d.DeviceType)
}

// MetricType 预测指标类型
type MetricType string

const (
	MetricImpressions MetricType = "impressions"
	MetricRevenue     MetricType = "revenue"
	MetricECPM        MetricType = "ecpm"
)

// ModelEntry 缓存的模型条目
type ModelEntry struct {
	Prophet   *model.Prophet
	TrainedAt time.Time
	Key       DimensionKey
	Metric    MetricType
}

// DimensionAggregator 按维度管理多个模型
type DimensionAggregator struct {
	mu      sync.RWMutex
	models  map[string]*ModelEntry // key: dim.String() + "|" + metric
	fetcher fetch.Fetcher
	trainMu sync.Mutex // 防止并发重训同一模型
}

// NewDimensionAggregator 创建聚合器
func NewDimensionAggregator(fetcher fetch.Fetcher) *DimensionAggregator {
	return &DimensionAggregator{
		models:  make(map[string]*ModelEntry),
		fetcher: fetcher,
	}
}

// modelKey 生成模型存储键
func modelKey(dim DimensionKey, metric MetricType) string {
	return dim.String() + "|" + string(metric)
}

// GetOrTrain 获取已有模型，不存在则训练新模型
func (a *DimensionAggregator) GetOrTrain(ctx context.Context, dim DimensionKey, metric MetricType) (*model.Prophet, error) {
	key := modelKey(dim, metric)

	a.mu.RLock()
	entry, ok := a.models[key]
	a.mu.RUnlock()

	if ok && time.Since(entry.TrainedAt) < 24*time.Hour {
		return entry.Prophet, nil
	}

	return a.train(ctx, dim, metric)
}

// Retrain 强制重训指定维度的模型
func (a *DimensionAggregator) Retrain(ctx context.Context, dim DimensionKey, metric MetricType) (*model.Prophet, error) {
	return a.train(ctx, dim, metric)
}

func (a *DimensionAggregator) train(ctx context.Context, dim DimensionKey, metric MetricType) (*model.Prophet, error) {
	a.trainMu.Lock()
	defer a.trainMu.Unlock()

	// Double-check after acquiring write lock
	key := modelKey(dim, metric)
	a.mu.RLock()
	entry, ok := a.models[key]
	a.mu.RUnlock()
	if ok && time.Since(entry.TrainedAt) < 24*time.Hour {
		return entry.Prophet, nil
	}

	records, err := a.fetcher.FetchHistory(ctx,
		[]string{dim.SlotID},
		[]string{dim.Country},
		[]string{dim.DeviceType},
		180,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch history for %s: %w", key, err)
	}

	trainData := recordsToTrainData(records, metric)
	prophet, err := model.Train(trainData)
	if err != nil {
		return nil, fmt.Errorf("train model for %s: %w", key, err)
	}

	a.mu.Lock()
	a.models[key] = &ModelEntry{
		Prophet:   prophet,
		TrainedAt: time.Now(),
		Key:       dim,
		Metric:    metric,
	}
	a.mu.Unlock()

	return prophet, nil
}

// TrainAll 训练所有维度组合（适用于每日 cron）
func (a *DimensionAggregator) TrainAll(ctx context.Context, dims []DimensionKey, metrics []MetricType) []error {
	type task struct {
		dim    DimensionKey
		metric MetricType
	}

	tasks := make([]task, 0, len(dims)*len(metrics))
	for _, d := range dims {
		for _, m := range metrics {
			tasks = append(tasks, task{d, m})
		}
	}

	// 并发训练，限制并发数
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	errCh := make(chan error, len(tasks))

	for _, t := range tasks {
		wg.Add(1)
		go func(t task) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			_, err := a.train(ctx, t.dim, t.metric)
			if err != nil {
				errCh <- err
			}
		}(t)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for e := range errCh {
		errs = append(errs, e)
	}
	return errs
}

// recordsToTrainData 将历史记录转为训练数据
func recordsToTrainData(records []fetch.DailyRecord, metric MetricType) []model.TrainData {
	// 按日期聚合（可能有多条相同 date 的记录）
	dateMap := make(map[string]float64)
	dateTime := make(map[string]time.Time)

	for _, r := range records {
		dateKey := r.Date.Format("2006-01-02")
		var val float64
		switch metric {
		case MetricImpressions:
			val = float64(r.Impressions)
		case MetricRevenue:
			val = r.Revenue
		case MetricECPM:
			if r.Impressions > 0 {
				val = r.Revenue / float64(r.Impressions) * 1000
			}
		}
		dateMap[dateKey] += val
		dateTime[dateKey] = r.Date
	}

	result := make([]model.TrainData, 0, len(dateMap))
	for k, v := range dateMap {
		result = append(result, model.TrainData{Date: dateTime[k], Value: v})
	}

	// 按日期排序
	sortTrainData(result)
	return result
}

// sortTrainData 对训练数据按日期升序排序
func sortTrainData(data []model.TrainData) {
	for i := 1; i < len(data); i++ {
		for j := i; j > 0 && data[j].Date.Before(data[j-1].Date); j-- {
			data[j], data[j-1] = data[j-1], data[j]
		}
	}
}

// ParseMetric 解析指标字符串
func ParseMetric(s string) (MetricType, error) {
	switch strings.ToLower(s) {
	case "impressions":
		return MetricImpressions, nil
	case "revenue":
		return MetricRevenue, nil
	case "ecpm":
		return MetricECPM, nil
	default:
		return "", fmt.Errorf("unknown metric: %s", s)
	}
}
