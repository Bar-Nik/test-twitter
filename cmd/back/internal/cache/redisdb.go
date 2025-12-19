package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	client *redis.Client
	// ctx    context.Context
}

func NewRedisClient(addr, password string, db int) *RedisClient {
	return &RedisClient{
		client: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       db,
			// DisableIdentity: true, // ← ПРАВИЛЬНОЕ написание
			// PoolSize:     10,
			// MinIdleConns: 5,
			// DialTimeout:  5 * time.Second,
			// ReadTimeout:  3 * time.Second,
			// WriteTimeout: 3 * time.Second,
		}),
		// ctx: context.Background(),
	}
}

func (r *RedisClient) Connect(ctx context.Context) error {
	err := r.client.Ping(ctx).Err()
	return err
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}

func (r *RedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return r.client.Set(ctx, key, value, expiration).Err()
}

func (r *RedisClient) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

func (r *RedisClient) GetDelete(ctx context.Context, key string) (string, error) {
	return r.client.GetDel(ctx, key).Result()
}

// AddToRight добавляет элемент в конец списка
func (r *RedisClient) AddToRight(ctx context.Context, key string, items ...string) error {
	return r.client.RPush(ctx, key, items).Err()
}

// GetList получает весь список
func (r *RedisClient) GetList(ctx context.Context, key string) ([]string, error) {
	return r.client.LRange(ctx, key, 0, -1).Result()
}

// RemoveElements удаляет все элементы по значению. 0-все
func (r *RedisClient) RemoveElements(ctx context.Context, key string, value string) (int64, error) {
	return r.client.LRem(ctx, key, 0, value).Result()
}
