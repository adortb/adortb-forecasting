// Command forecasting 启动预测引擎 HTTP 服务
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adortb/adortb-forecasting/internal/aggregator"
	"github.com/adortb/adortb-forecasting/internal/api"
	"github.com/adortb/adortb-forecasting/internal/cache"
	"github.com/adortb/adortb-forecasting/internal/fetch"
	"github.com/adortb/adortb-forecasting/internal/metrics"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := loadConfig()

	// 初始化 ClickHouse fetcher
	var fetcher fetch.Fetcher
	if cfg.ClickHouseDSN != "" {
		chFetcher, err := fetch.NewCHFetcher(fetch.ClickHouseConfig{DSN: cfg.ClickHouseDSN})
		if err != nil {
			slog.Error("clickhouse init failed", "err", err)
			os.Exit(1)
		}
		defer chFetcher.Close()
		fetcher = chFetcher
	} else {
		slog.Warn("CLICKHOUSE_DSN not set, using mock fetcher (dev mode)")
		fetcher = &fetch.MockFetcher{}
	}

	// 初始化 Redis 缓存
	var cacheClient cache.Cache
	if cfg.RedisAddr != "" {
		cacheClient = cache.NewCacheClient(cache.Config{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       0,
			TTL:      time.Hour,
		})
	} else {
		slog.Warn("REDIS_ADDR not set, using nop cache (dev mode)")
		cacheClient = cache.NopCache{}
	}

	// 初始化聚合器和 HTTP 处理器
	agg := aggregator.NewDimensionAggregator(fetcher)
	handler := api.NewHandler(agg, cacheClient)
	m := metrics.NewSimpleMetrics()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.Handle("GET /metrics", m.Handler())

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      m.Middleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// 启动每日重训 cron（简单 goroutine ticker）
	if cfg.EnableDailyCron {
		go runDailyCron(agg)
	}

	// 优雅启动
	go func() {
		slog.Info("forecasting service starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	// 等待信号优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server shutdown error", "err", err)
	}
	slog.Info("forecasting service stopped")
}

// config 运行时配置
type config struct {
	Port            string
	ClickHouseDSN   string
	RedisAddr       string
	RedisPassword   string
	EnableDailyCron bool
}

func loadConfig() config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8100"
	}
	return config{
		Port:            port,
		ClickHouseDSN:   os.Getenv("CLICKHOUSE_DSN"),
		RedisAddr:       os.Getenv("REDIS_ADDR"),
		RedisPassword:   os.Getenv("REDIS_PASSWORD"),
		EnableDailyCron: os.Getenv("DISABLE_DAILY_CRON") == "",
	}
}

// runDailyCron 每日凌晨重训所有已知维度组合（简化实现：每 24h ticker）
func runDailyCron(agg *aggregator.DimensionAggregator) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		slog.Info("daily cron: starting model retraining")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		// 实际使用时从数据库或配置加载维度列表，此处仅示意
		errs := agg.TrainAll(ctx, nil, []aggregator.MetricType{
			aggregator.MetricImpressions,
			aggregator.MetricRevenue,
		})
		cancel()
		if len(errs) > 0 {
			for _, e := range errs {
				slog.Error("daily cron training error", "err", e)
			}
		} else {
			slog.Info("daily cron: model retraining completed")
		}
	}
}
