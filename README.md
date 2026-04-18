# adortb-forecasting

十期服务。Prophet-like 时序分解预测引擎，用于广告投放预算规划、转化量预测和流量趋势预测。

## 算法概述

### 分解模型

Prophet 时序分解公式：

```
y(t) = trend(t) + seasonality(t) + holiday(t) + ε
```

每个分量独立拟合，组合后生成带置信区间的预测。

### 趋势分量（Piecewise Linear Trend）

分段线性趋势，支持自动检测趋势变更点：

```
trend(t) = (k + Σ δ_j · 1[t ≥ s_j]) · t + (m - Σ δ_j · s_j · 1[t ≥ s_j])

其中：
  k   = 初始斜率
  m   = 初始截距
  s_j = 第j个变更点时刻（归一化 [0,1]）
  δ_j = 第j个变更点的斜率变化量
```

变更点均匀分布于时间序列的 [10%, 90%] 区间。变更点斜率 δ 通过 **带L2正则化的最小二乘**（Cholesky分解）求解：

```
(AᵀA + λI) δ = Aᵀr    λ = 0.05
```

### 季节性分量（Fourier Series）

使用 Fourier 级数建模周期性：

```
seasonality(t) = Σ_{j=1}^{K} [a_j · cos(2πjt/P) + b_j · sin(2πjt/P)]

周季节性：P = 7天，K = 3（6个系数）
年季节性：P = 365.25天，K = 10（20个系数）
```

系数通过最小二乘（含 1e-6 正则化）拟合：`FitSeasonality(ts, residuals, period, K)`

### 节假日分量

`holiday.go` 建模各国法定节假日对时序的阶跃影响，拟合对应的调整量。

### 置信区间

通过扰动变更点位置进行 **Bootstrap 仿真**，生成未来预测的置信区间（默认 95%）。

## 模型可靠性

- `IsReliable()`: MAPE < 30% 且训练数据足够
- `IsLowConfidence()`: 训练数据不足（冷启动保护）

## 快速开始

```bash
go build -o bin/forecasting ./cmd/forecasting
./bin/forecasting -port 8081

# 请求示例
curl -X POST http://localhost:8081/v1/forecast \
  -d '{"metric":"revenue","dimensions":{"app_id":"app_001"},"horizon_days":30}'
```

## API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/v1/forecast` | 预测未来 N 天 |
| GET  | `/metrics` | Prometheus 指标 |

## 模型评估指标

| 指标 | 说明 |
|------|------|
| MAPE | 平均绝对百分比误差，< 30% 为可靠 |
| RMSE | 均方根误差 |
| Coverage | 置信区间覆盖率（目标 ≥ 95%） |

## 技术栈

- **语言**: Go
- **数据源**: ClickHouse（历史指标）
- **缓存**: Redis（预测结果缓存）
- **监控**: Prometheus

## 目录结构

```
adortb-forecasting/
├── cmd/forecasting/        # 服务入口
├── internal/
│   ├── api/                # HTTP handler
│   ├── aggregator/         # 多维度数据聚合
│   ├── cache/              # Redis 缓存
│   ├── fetch/              # ClickHouse 数据拉取
│   ├── metrics/            # Prometheus 指标
│   └── model/
│       ├── prophet.go      # 整体模型封装
│       ├── trend.go        # 分段线性趋势 + Cholesky OLS
│       ├── seasonality.go  # Fourier 季节性
│       ├── holiday.go      # 节假日效应
│       ├── fit.go          # 模型拟合流程
│       └── predict.go      # 预测生成
```
