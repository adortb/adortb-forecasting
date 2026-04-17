// Package api 提供 HTTP REST API 处理器
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/adortb/adortb-forecasting/internal/aggregator"
	"github.com/adortb/adortb-forecasting/internal/cache"
	"github.com/adortb/adortb-forecasting/internal/model"
)

// Handler HTTP 处理器
type Handler struct {
	agg   *aggregator.DimensionAggregator
	cache cache.Cache
}

// NewHandler 创建处理器
func NewHandler(agg *aggregator.DimensionAggregator, c cache.Cache) *Handler {
	return &Handler{agg: agg, cache: c}
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/forecast", h.handleForecast)
	mux.HandleFunc("POST /v1/forecast/budget", h.handleBudget)
	mux.HandleFunc("POST /v1/forecast/inventory", h.handleInventory)
	mux.HandleFunc("POST /v1/models/{id}/retrain", h.handleRetrain)
	mux.HandleFunc("GET /health", h.handleHealth)
}

// ─── Request/Response 结构 ───────────────────────────────────────────────────

// ForecastRequest 预测请求
type ForecastRequest struct {
	Metric          string              `json:"metric"`
	Dimensions      map[string][]string `json:"dimensions"`
	HorizonDays     int                 `json:"horizon_days"`
	ConfidenceLevel float64             `json:"confidence_level"`
}

// ForecastPointDTO 预测点响应
type ForecastPointDTO struct {
	Date    string  `json:"date"`
	Value   float64 `json:"value"`
	Lower80 float64 `json:"lower_80"`
	Upper80 float64 `json:"upper_80"`
	Lower95 float64 `json:"lower_95"`
	Upper95 float64 `json:"upper_95"`
}

// ForecastResponse 预测响应
type ForecastResponse struct {
	Forecast  []ForecastPointDTO `json:"forecast"`
	ModelInfo model.ModelInfo    `json:"model_info"`
}

// BudgetRequest 预算反推请求
type BudgetRequest struct {
	Budget     float64             `json:"budget"`
	Dimensions map[string][]string `json:"dimensions"`
	StartDate  string              `json:"start_date"` // "2026-04-19"
	Days       int                 `json:"days"`
}

// BudgetResponse 预算反推响应
type BudgetResponse struct {
	EstimatedImpressions int64   `json:"estimated_impressions"`
	EstimatedECPM        float64 `json:"estimated_ecpm"`
	DaysToSpend          int     `json:"days_to_spend"`
}

// InventoryRequest 库存检查请求
type InventoryRequest struct {
	Dimensions  map[string][]string `json:"dimensions"`
	StartDate   string              `json:"start_date"`
	EndDate     string              `json:"end_date"`
	Impressions int64               `json:"impressions"` // 期望量
}

// InventoryResponse 库存检查响应
type InventoryResponse struct {
	Available      int64   `json:"available"`
	Requested      int64   `json:"requested"`
	CanFulfill     bool    `json:"can_fulfill"`
	Confidence     float64 `json:"confidence"`
	FulfillmentPct float64 `json:"fulfillment_pct"`
}

// ─── 处理器 ───────────────────────────────────────────────────────────────────

func (h *Handler) handleForecast(w http.ResponseWriter, r *http.Request) {
	var req ForecastRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateForecastRequest(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 生成缓存键
	cacheKey := cache.ForecastKey(req.Metric, dimensionsKey(req.Dimensions), req.HorizonDays)
	var cached ForecastResponse
	if ok, _ := h.cache.Get(r.Context(), cacheKey, &cached); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	// 获取或训练模型（按维度组合展开，若多个则聚合）
	resp, err := h.buildForecastResponse(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_ = h.cache.Set(r.Context(), cacheKey, resp)
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) buildForecastResponse(ctx context.Context, req ForecastRequest) (*ForecastResponse, error) {
	metric, err := aggregator.ParseMetric(req.Metric)
	if err != nil {
		return nil, err
	}

	dims := expandDimensions(req.Dimensions)
	if len(dims) == 0 {
		return nil, fmt.Errorf("no valid dimensions specified")
	}

	confidenceLevel := req.ConfidenceLevel
	if confidenceLevel <= 0 {
		confidenceLevel = 0.8
	}

	// 按维度分别预测后聚合
	aggregated := make(map[string]*ForecastPointDTO)
	var lastInfo model.ModelInfo

	for _, dim := range dims {
		prophet, err := h.agg.GetOrTrain(ctx, dim, metric)
		if err != nil {
			return nil, fmt.Errorf("get model for %s: %w", dim, err)
		}

		points, info := prophet.Forecast(time.Now(), req.HorizonDays, confidenceLevel)
		lastInfo = info

		for _, p := range points {
			dateStr := p.Date.Format("2006-01-02")
			if existing, ok := aggregated[dateStr]; ok {
				existing.Value += p.Value
				existing.Lower80 += p.Lower80
				existing.Upper80 += p.Upper80
				existing.Lower95 += p.Lower95
				existing.Upper95 += p.Upper95
			} else {
				aggregated[dateStr] = &ForecastPointDTO{
					Date:    dateStr,
					Value:   p.Value,
					Lower80: p.Lower80,
					Upper80: p.Upper80,
					Lower95: p.Lower95,
					Upper95: p.Upper95,
				}
			}
		}
	}

	result := sortedForecastPoints(aggregated)
	return &ForecastResponse{Forecast: result, ModelInfo: lastInfo}, nil
}

func (h *Handler) handleBudget(w http.ResponseWriter, r *http.Request) {
	var req BudgetRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Budget <= 0 {
		writeError(w, http.StatusBadRequest, "budget must be positive")
		return
	}
	if req.Days <= 0 {
		req.Days = 30
	}

	// 预测该区间内的 impression 和 revenue，反推 eCPM
	dims := expandDimensions(req.Dimensions)
	if len(dims) == 0 {
		writeError(w, http.StatusBadRequest, "no valid dimensions")
		return
	}

	var totalImpressions, totalRevenue float64
	for _, dim := range dims {
		impProphet, err := h.agg.GetOrTrain(r.Context(), dim, aggregator.MetricImpressions)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		revProphet, err := h.agg.GetOrTrain(r.Context(), dim, aggregator.MetricRevenue)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		startDate, _ := time.Parse("2006-01-02", req.StartDate)
		if startDate.IsZero() {
			startDate = time.Now()
		}

		impPoints, _ := impProphet.Forecast(startDate, req.Days, 0.8)
		revPoints, _ := revProphet.Forecast(startDate, req.Days, 0.8)

		for _, p := range impPoints {
			totalImpressions += p.Value
		}
		for _, p := range revPoints {
			totalRevenue += p.Value
		}
	}

	var ecpm float64
	if totalImpressions > 0 {
		ecpm = totalRevenue / totalImpressions * 1000
	}

	var estimatedImpressions int64
	if ecpm > 0 {
		estimatedImpressions = int64(req.Budget / ecpm * 1000)
	}

	writeJSON(w, http.StatusOK, BudgetResponse{
		EstimatedImpressions: estimatedImpressions,
		EstimatedECPM:        ecpm,
		DaysToSpend:          req.Days,
	})
}

func (h *Handler) handleInventory(w http.ResponseWriter, r *http.Request) {
	var req InventoryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	startDate, _ := time.Parse("2006-01-02", req.StartDate)
	endDate, _ := time.Parse("2006-01-02", req.EndDate)
	if startDate.IsZero() {
		startDate = time.Now()
	}
	if endDate.IsZero() {
		endDate = startDate.Add(30 * 24 * time.Hour)
	}
	days := int(endDate.Sub(startDate).Hours()/24) + 1
	if days <= 0 {
		days = 30
	}

	dims := expandDimensions(req.Dimensions)
	var totalAvailable float64

	for _, dim := range dims {
		prophet, err := h.agg.GetOrTrain(r.Context(), dim, aggregator.MetricImpressions)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		points, _ := prophet.Forecast(startDate, days, 0.8)
		for _, p := range points {
			totalAvailable += p.Lower80 // 保守估计使用 80% 下界
		}
	}

	canFulfill := totalAvailable >= float64(req.Impressions)
	fulfillPct := 0.0
	if req.Impressions > 0 {
		fulfillPct = totalAvailable / float64(req.Impressions)
		if fulfillPct > 1 {
			fulfillPct = 1
		}
	}

	writeJSON(w, http.StatusOK, InventoryResponse{
		Available:      int64(totalAvailable),
		Requested:      req.Impressions,
		CanFulfill:     canFulfill,
		Confidence:     0.8,
		FulfillmentPct: fulfillPct,
	})
}

func (h *Handler) handleRetrain(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	parts := strings.SplitN(idStr, "|", 4)
	if len(parts) < 4 {
		writeError(w, http.StatusBadRequest, "model id must be: slotID|country|deviceType|metric")
		return
	}
	dim := aggregator.DimensionKey{SlotID: parts[0], Country: parts[1], DeviceType: parts[2]}
	metric, err := aggregator.ParseMetric(parts[3])
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err = h.agg.Retrain(r.Context(), dim, metric)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "retrained", "model_id": idStr})
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "adortb-forecasting"})
}

// ─── 工具函数 ─────────────────────────────────────────────────────────────────

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func validateForecastRequest(req *ForecastRequest) error {
	if req.Metric == "" {
		return fmt.Errorf("metric is required")
	}
	if req.HorizonDays <= 0 || req.HorizonDays > 365 {
		return fmt.Errorf("horizon_days must be between 1 and 365")
	}
	if len(req.Dimensions) == 0 {
		return fmt.Errorf("dimensions is required")
	}
	return nil
}

// expandDimensions 将维度 map 展开为所有组合的 DimensionKey 列表
func expandDimensions(dims map[string][]string) []aggregator.DimensionKey {
	slotIDs := dims["slot_id"]
	countries := dims["country"]
	deviceTypes := dims["device_type"]

	if len(slotIDs) == 0 {
		slotIDs = []string{"*"}
	}
	if len(countries) == 0 {
		countries = []string{"*"}
	}
	if len(deviceTypes) == 0 {
		deviceTypes = []string{"*"}
	}

	var keys []aggregator.DimensionKey
	for _, s := range slotIDs {
		for _, c := range countries {
			for _, d := range deviceTypes {
				keys = append(keys, aggregator.DimensionKey{
					SlotID:     s,
					Country:    c,
					DeviceType: d,
				})
			}
		}
	}
	return keys
}

// dimensionsKey 生成维度的字符串键（用于缓存）
func dimensionsKey(dims map[string][]string) string {
	parts := make([]string, 0, len(dims)*2)
	for k, vs := range dims {
		sort.Strings(vs)
		parts = append(parts, k+"="+strings.Join(vs, ","))
	}
	sort.Strings(parts)
	return strings.Join(parts, ";")
}

// sortedForecastPoints 将 map 转为按日期排序的切片
func sortedForecastPoints(m map[string]*ForecastPointDTO) []ForecastPointDTO {
	dates := make([]string, 0, len(m))
	for d := range m {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	result := make([]ForecastPointDTO, 0, len(dates))
	for _, d := range dates {
		result = append(result, *m[d])
	}
	return result
}
