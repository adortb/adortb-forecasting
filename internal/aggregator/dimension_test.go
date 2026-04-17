package aggregator

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/adortb/adortb-forecasting/internal/fetch"
)

func makeMockRecords(n int) []fetch.DailyRecord {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	records := make([]fetch.DailyRecord, n)
	for i := range records {
		records[i] = fetch.DailyRecord{
			Date:        base.Add(time.Duration(i) * 24 * time.Hour),
			SlotID:      "slot_1",
			Country:     "US",
			DeviceType:  "mobile",
			Impressions: int64(10000 + i*100),
			Revenue:     float64(100 + i),
		}
	}
	return records
}

func TestGetOrTrain_Basic(t *testing.T) {
	mock := &fetch.MockFetcher{Records: makeMockRecords(60)}
	agg := NewDimensionAggregator(mock)
	dim := DimensionKey{SlotID: "slot_1", Country: "US", DeviceType: "mobile"}

	prophet, err := agg.GetOrTrain(context.Background(), dim, MetricImpressions)
	if err != nil {
		t.Fatalf("GetOrTrain error: %v", err)
	}
	if prophet == nil {
		t.Fatal("prophet is nil")
	}
}

func TestGetOrTrain_CacheHit(t *testing.T) {
	mock := &fetch.MockFetcher{Records: makeMockRecords(60)}
	agg := NewDimensionAggregator(mock)
	dim := DimensionKey{SlotID: "slot_1", Country: "US", DeviceType: "mobile"}

	p1, _ := agg.GetOrTrain(context.Background(), dim, MetricImpressions)
	p2, _ := agg.GetOrTrain(context.Background(), dim, MetricImpressions)
	if p1 != p2 {
		t.Error("expected same model instance on cache hit")
	}
}

func TestRecordsToTrainData_Impressions(t *testing.T) {
	records := makeMockRecords(5)
	data := recordsToTrainData(records, MetricImpressions)
	if len(data) != 5 {
		t.Errorf("want 5 data points, got %d", len(data))
	}
	if data[0].Value != float64(records[0].Impressions) {
		t.Errorf("value mismatch: want %f, got %f", float64(records[0].Impressions), data[0].Value)
	}
}

func TestRecordsToTrainData_Revenue(t *testing.T) {
	records := makeMockRecords(5)
	data := recordsToTrainData(records, MetricRevenue)
	if data[0].Value != records[0].Revenue {
		t.Errorf("revenue mismatch: want %f, got %f", records[0].Revenue, data[0].Value)
	}
}

func TestRecordsToTrainData_ECPM(t *testing.T) {
	records := []fetch.DailyRecord{
		{Date: time.Now(), SlotID: "s", Country: "US", DeviceType: "mobile",
			Impressions: 1000, Revenue: 2.0},
	}
	data := recordsToTrainData(records, MetricECPM)
	want := 2.0 / 1000 * 1000 // 2.0 eCPM
	if math.Abs(data[0].Value-want) > 1e-9 {
		t.Errorf("eCPM: want %f, got %f", want, data[0].Value)
	}
}

func TestDimensionKey_String(t *testing.T) {
	dim := DimensionKey{SlotID: "s1", Country: "US", DeviceType: "mobile"}
	got := dim.String()
	want := "s1|US|mobile"
	if got != want {
		t.Errorf("String(): want %q, got %q", want, got)
	}
}

func TestParseMetric(t *testing.T) {
	cases := []struct {
		input string
		want  MetricType
		isErr bool
	}{
		{"impressions", MetricImpressions, false},
		{"revenue", MetricRevenue, false},
		{"ecpm", MetricECPM, false},
		{"IMPRESSIONS", MetricImpressions, false},
		{"unknown", "", true},
	}
	for _, c := range cases {
		got, err := ParseMetric(c.input)
		if c.isErr && err == nil {
			t.Errorf("ParseMetric(%q): expected error", c.input)
		}
		if !c.isErr && got != c.want {
			t.Errorf("ParseMetric(%q): want %q, got %q", c.input, c.want, got)
		}
	}
}
