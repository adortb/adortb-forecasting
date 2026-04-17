// Package cache 提供预测结果的 Redis 缓存（小时级 TTL）
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultTTL = time.Hour

// CacheClient Redis 缓存客户端
type CacheClient struct {
	rdb *redis.Client
	ttl time.Duration
}

// Config Redis 配置
type Config struct {
	Addr     string
	Password string
	DB       int
	TTL      time.Duration
}

// NewCacheClient 创建 Redis 缓存客户端
func NewCacheClient(cfg Config) *CacheClient {
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = defaultTTL
	}
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		PoolSize:     10,
	})
	return &CacheClient{rdb: rdb, ttl: ttl}
}

// Get 从缓存读取预测结果（未命中返回 nil, false）
func (c *CacheClient) Get(ctx context.Context, key string, dst any) (bool, error) {
	val, err := c.rdb.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cache get %s: %w", key, err)
	}
	if err := json.Unmarshal(val, dst); err != nil {
		return false, fmt.Errorf("cache unmarshal %s: %w", key, err)
	}
	return true, nil
}

// Set 写入缓存
func (c *CacheClient) Set(ctx context.Context, key string, val any) error {
	b, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("cache marshal: %w", err)
	}
	return c.rdb.Set(ctx, key, b, c.ttl).Err()
}

// Delete 删除缓存键
func (c *CacheClient) Delete(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

// ForecastKey 生成预测结果的缓存键（基于请求参数哈希）
func ForecastKey(metric, dimensions string, horizonDays int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d", metric, dimensions, horizonDays)))
	return fmt.Sprintf("forecast:v1:%x", h[:8])
}

// Close 关闭 Redis 连接
func (c *CacheClient) Close() error {
	return c.rdb.Close()
}

// NopCache 空 cache（用于测试或禁用缓存）
type NopCache struct{}

func (NopCache) Get(_ context.Context, _ string, _ any) (bool, error) { return false, nil }
func (NopCache) Set(_ context.Context, _ string, _ any) error         { return nil }
func (NopCache) Delete(_ context.Context, _ string) error             { return nil }

// Cache 统一缓存接口
type Cache interface {
	Get(ctx context.Context, key string, dst any) (bool, error)
	Set(ctx context.Context, key string, val any) error
	Delete(ctx context.Context, key string) error
}
