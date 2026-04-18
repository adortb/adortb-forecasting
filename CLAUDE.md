# CLAUDE.md — adortb-forecasting

## 项目角色

十期服务：Prophet-like 时序预测。为预算规划和趋势预警提供未来 7~90 天的预测，对预测精度（MAPE）敏感。

## 关键函数与复杂度

| 函数 | 文件 | 复杂度 | 说明 |
|------|------|--------|------|
| `FitTrend` | `model/trend.go` | O(T·C²)，T=时间步，C=变更点数 | 分段线性趋势拟合 |
| `solveCholesky` | `model/trend.go` | O(C³) | 对称正定线性方程组求解 |
| `FitSeasonality` | `model/seasonality.go` | O(T·K²) | Fourier 最小二乘拟合 |
| `leastSquares` | `model/seasonality.go` | O(T·K²) | 正规方程 + Cholesky |
| `FitModel` | `model/fit.go` | O(T·(C²+K²)) | 完整模型拟合 |
| `Trend.At(t)` | `model/trend.go` | O(C) | 查询t时刻趋势值 |
| `SeasonalityComponent.At(t)` | `model/seasonality.go` | O(K) | Fourier 求值 |

## 模型拟合规范

- 变更点数量 `nChangePoints` 不应超过数据量的 1/4，否则过拟合
- 变更点斜率 L2 正则化系数 `λ=0.05`（固定），不需修改
- 季节性正则化 `1e-6`（防奇异），不需修改
- 季节性分量：**周季节K=3，年季节K=10**，这是经验最优值
- 变更点均匀分布于 [10%, 90%] 时间区间，避免边界效应

## 训练数据要求

- 训练最少数据量：建议 ≥ 60 天（否则 `IsLowConfidence()=true`）
- 年季节性需要 ≥ 2 年数据才可靠
- 缺失值需在 `aggregator/` 层做插值，不能传入 NaN

## Cholesky 分解注意事项

`solveCholesky` 要求矩阵正定，已有 `max(sum, 1e-10)` 防止数值不稳定：
- 不要去掉正则化项 `λI`
- 如遇到大规模变更点（C > 50），需验证精度

## 预测流程说明

```
Prophet.Forecast(startDate, horizonDays, confidenceLevel)
  ├── Predict(fit, startDate, cfg)        // 主预测
  │   ├── trend.At(t) 每个时间步
  │   ├── seasonality.At(t) 每个时间步
  │   └── holiday 调整
  └── GetModelInfo(fit)                   // 模型元信息
```

## 并发与缓存

- 预测结果存 Redis，key = `forecast:{metric}:{dims_hash}:{horizon}`
- 同一 key 缓存 TTL 建议 1 小时
- 模型拟合计算密集，不应在请求路径上同步执行，应后台异步拟合

## 评估指标目标

| 指标 | 目标 |
|------|------|
| MAPE（验证集） | < 30%（否则标记 not reliable）|
| P95 预测接口延迟 | < 500ms（缓存命中 < 10ms）|

## 测试

```bash
go test -race ./...
go test ./internal/model/ -v -run TestFitTrend
go test ./internal/model/ -v -run TestSeasonality
go test ./internal/model/ -v -run TestProphet
```

关键测试：
- `model/trend_test.go` — 线性回归、变更点拟合
- `model/seasonality_test.go` — Fourier 系数、周/年季节性
- `model/prophet_test.go` — 端到端预测正确性
- `model/fit_test.go` — FitModel 与真实数据对比
- `model/holiday_test.go` — 节假日效应
