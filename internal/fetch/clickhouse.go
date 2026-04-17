// Package fetch 负责从 ClickHouse 拉取历史数据
package fetch

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

// DailyRecord 按天聚合的记录
type DailyRecord struct {
	Date       time.Time
	SlotID     string
	Country    string
	DeviceType string
	Impressions int64
	Revenue    float64
}

// ClickHouseConfig ClickHouse 连接配置
type ClickHouseConfig struct {
	DSN string // clickhouse://user:pass@host:9000/db
}

// Fetcher 数据拉取器接口（便于 mock 测试）
type Fetcher interface {
	FetchHistory(ctx context.Context, slotIDs, countries, deviceTypes []string, days int) ([]DailyRecord, error)
}

// CHFetcher ClickHouse 实现
type CHFetcher struct {
	db *sql.DB
}

// NewCHFetcher 创建 ClickHouse 拉取器
func NewCHFetcher(cfg ClickHouseConfig) (*CHFetcher, error) {
	db, err := sql.Open("clickhouse", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("clickhouse open: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(10 * time.Minute)
	return &CHFetcher{db: db}, nil
}

// FetchHistory 拉取历史聚合数据
func (f *CHFetcher) FetchHistory(ctx context.Context, slotIDs, countries, deviceTypes []string, days int) ([]DailyRecord, error) {
	query := `
		SELECT event_date, slot_id, country, device_type,
		       COUNT(*) AS impressions,
		       SUM(price) AS revenue
		FROM events
		WHERE event_type = 'impression'
		  AND event_date >= today() - ?
		  AND slot_id IN (?)
		  AND country IN (?)
		  AND device_type IN (?)
		GROUP BY event_date, slot_id, country, device_type
		ORDER BY event_date
	`

	rows, err := f.db.QueryContext(ctx, query, days,
		slotIDs, countries, deviceTypes)
	if err != nil {
		return nil, fmt.Errorf("clickhouse query: %w", err)
	}
	defer rows.Close()

	var records []DailyRecord
	for rows.Next() {
		var r DailyRecord
		if err := rows.Scan(&r.Date, &r.SlotID, &r.Country, &r.DeviceType,
			&r.Impressions, &r.Revenue); err != nil {
			return nil, fmt.Errorf("clickhouse scan: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// Close 关闭连接
func (f *CHFetcher) Close() error {
	return f.db.Close()
}

// MockFetcher 用于单元测试的 mock 实现
type MockFetcher struct {
	Records []DailyRecord
}

// FetchHistory 返回预设数据
func (m *MockFetcher) FetchHistory(_ context.Context, _, _, _ []string, _ int) ([]DailyRecord, error) {
	return m.Records, nil
}
