# Architecture — adortb-forecasting

## 系统概览

```
┌────────────────────────────────────────────────────────┐
│                    HTTP API Server                      │
│              POST /v1/forecast                          │
└──────────────────────┬─────────────────────────────────┘
                       │
          ┌────────────▼────────────┐
          │   Redis Cache Check      │ 命中 → 直接返回
          └────────────┬────────────┘
                   未命中
                       │
          ┌────────────▼────────────┐
          │  Aggregator / Fetcher   │ ClickHouse 历史数据
          └────────────┬────────────┘
                       │
          ┌────────────▼────────────┐
          │      FitModel()         │ 模型拟合
          │  trend + seasonality    │
          │  + holiday              │
          └────────────┬────────────┘
                       │
          ┌────────────▼────────────┐
          │    Prophet.Forecast()   │ 生成预测点
          └────────────┬────────────┘
                       │
          ┌────────────▼────────────┐
          │   Redis Cache Write     │ 写入缓存
          └────────────┬────────────┘
                       │
                   预测结果返回
```

## 训练流程（模型拟合）

```
历史时间序列 [{t, y}]
    │
    ▼
┌──────────────────────────────────────────────────────┐
│                     FitModel()                        │
│                                                       │
│  Step 1: 时间归一化（t → [0,1]）                      │
│                                                       │
│  Step 2: FitTrend(ts, ys, nChangePoints)             │
│    ├── linearRegression(ts, ys) → k, m               │
│    ├── makeChangePointTimes(ts, n) → [s_1..s_n]      │
│    └── fitChangePointDeltas() → [δ_1..δ_n]           │
│         └── (AᵀA + λI)δ = Aᵀr  [Cholesky OLS]      │
│                                                       │
│  Step 3: 去除趋势残差                                  │
│          residuals[i] = y[i] - trend.At(t[i])        │
│                                                       │
│  Step 4: FitSeasonalities(ts, residuals)             │
│    ├── FitSeasonality(P=7,  K=3)  → 周季节 6个系数   │
│    └── FitSeasonality(P=365.25, K=10) → 年季节 20系数│
│         └── leastSquares(X_fourier, residuals)        │
│                                                       │
│  Step 5: FitHoliday(ts, residuals2)                  │
│          残差 = residuals - seasonality.At(t)         │
│                                                       │
│  Step 6: 计算验证集 MAPE                              │
│          Reliable = MAPE < 0.30                       │
└──────────────────────────────────────────────────────┘
    │
    ▼
FitResult { Trend, Seasonality, Holiday, MAPE, Reliable }
```

## 推理流程时序图

```
Client          API Handler         Cache          FitModel        Predict
  │                  │                 │               │               │
  │─ POST /forecast ►│                 │               │               │
  │                  │─ Get(key) ─────►│               │               │
  │                  │◄── miss ────────│               │               │
  │                  │─ FetchHistory() (ClickHouse)    │               │
  │                  │─ FitModel() ────────────────────►               │
  │                  │◄─── FitResult ──────────────────│               │
  │                  │─ Forecast() ────────────────────────────────────►
  │                  │◄─ []ForecastPoint ──────────────────────────────│
  │                  │─ Set(key, result) ──────────────►               │
  │◄─ JSON response ─│                 │               │               │
```

## 数据输入输出

### 输入数据（ClickHouse）

```sql
SELECT date, SUM(metric_value) as y
FROM adortb_metrics
WHERE app_id = ? AND metric = ?
  AND date >= ?
GROUP BY date
ORDER BY date
```

| 字段 | 说明 |
|------|------|
| date | 日期（天粒度） |
| y | 指标值（revenue / conversions / clicks） |

### 输出数据

```json
{
  "points": [
    {
      "date": "2026-04-19",
      "yhat": 12500.0,
      "yhat_lower": 11200.0,
      "yhat_upper": 13800.0,
      "trend": 11000.0,
      "seasonality": 1500.0
    }
  ],
  "model_info": {
    "mape": 0.12,
    "reliable": true,
    "low_confidence": false,
    "change_points": 5
  }
}
```

## 模型结构图

```
y(t) = trend(t) + seasonality(t) + holiday(t) + ε

trend(t):     ━━━━╱━━━━━━╱━━━━━━━╱━━━━  （分段线性，3~5个变更点）
                  s₁      s₂      s₃

seasonality:  ∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿  Fourier级数
              ├── 周季节（P=7, K=3）
              └── 年季节（P=365.25, K=10）

holiday:      ████          ████        （节假日脉冲）
```

## 评估指标

| 指标 | 计算公式 | 目标 |
|------|----------|------|
| MAPE | mean(|y-ŷ|/y) | < 30% |
| RMSE | √(mean((y-ŷ)²)) | 越小越好 |
| 置信区间覆盖率 | count(y ∈ [lower,upper]) / N | ≥ 95% |

## 依赖关系

```
adortb-forecasting
├── ClickHouse  （历史指标数据，按天聚合）
└── Redis       （预测结果缓存，TTL=1h）
```
