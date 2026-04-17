// Package metrics 提供 Prometheus 指标
package metrics

import (
	"net/http"
	"strconv"
	"time"
)

// SimpleMetrics 简单计数器（不依赖外部 Prometheus 库，便于轻量部署）
type SimpleMetrics struct {
	requestCount  map[string]int64
	requestLatency map[string][]float64
	mu            chan struct{} // 用 chan 实现简单互斥
}

// NewSimpleMetrics 创建指标收集器
func NewSimpleMetrics() *SimpleMetrics {
	m := &SimpleMetrics{
		requestCount:  make(map[string]int64),
		requestLatency: make(map[string][]float64),
		mu:            make(chan struct{}, 1),
	}
	m.mu <- struct{}{}
	return m
}

func (m *SimpleMetrics) lock()   { <-m.mu }
func (m *SimpleMetrics) unlock() { m.mu <- struct{}{} }

// RecordRequest 记录请求指标
func (m *SimpleMetrics) RecordRequest(path string, statusCode int, latency time.Duration) {
	key := path + ":" + strconv.Itoa(statusCode)
	m.lock()
	m.requestCount[key]++
	m.requestLatency[key] = append(m.requestLatency[key], latency.Seconds()*1000) // ms
	m.unlock()
}

// Handler 返回 /metrics 端点处理函数
func (m *SimpleMetrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")

		m.lock()
		defer m.unlock()

		for key, count := range m.requestCount {
			_, _ = w.Write([]byte("http_requests_total{path=\"" + key + "\"} " + strconv.FormatInt(count, 10) + "\n"))
		}
		for key, latencies := range m.requestLatency {
			p99 := percentile(latencies, 0.99)
			_, _ = w.Write([]byte("http_request_duration_ms_p99{path=\"" + key + "\"} " + strconv.FormatFloat(p99, 'f', 2, 64) + "\n"))
		}
	}
}

// Middleware 计时 + 指标中间件
func (m *SimpleMetrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		m.RecordRequest(r.URL.Path, rw.status, time.Since(start))
	})
}

// responseWriter 包装 http.ResponseWriter 捕获状态码
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// percentile 计算百分位数（假设已排序或直接近似计算）
func percentile(data []float64, p float64) float64 {
	if len(data) == 0 {
		return 0
	}
	// 简单近似：排序后取分位
	sorted := make([]float64, len(data))
	copy(sorted, data)
	sortFloats(sorted)
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

func sortFloats(a []float64) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}
