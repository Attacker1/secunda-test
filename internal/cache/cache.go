// Package cache — обёртка над Redis для кеширования списка задач команды.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"test-task/internal/domain"
)

// Cache абстрагирует операции кеша, чтобы сервисный слой можно было
// тестировать без реального Redis.
type Cache interface {
	GetTasks(ctx context.Context, key string) ([]domain.Task, bool, error)
	SetTasks(ctx context.Context, key string, tasks []domain.Task) error
	InvalidateTeam(ctx context.Context, teamID int64) error
}

type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedis(client *redis.Client, ttl time.Duration) *RedisCache {
	return &RedisCache{client: client, ttl: ttl}
}

func TasksKey(f domain.TaskFilter) string {
	status := "all"
	if f.Status != nil {
		status = string(*f.Status)
	}
	assignee := "all"
	if f.AssigneeID != nil {
		assignee = fmt.Sprintf("%d", *f.AssigneeID)
	}
	return fmt.Sprintf("tasks:team:%d:status:%s:assignee:%s:limit:%d:offset:%d",
		f.TeamID, status, assignee, f.Limit, f.Offset)
}

func teamSetKey(teamID int64) string {
	return fmt.Sprintf("tasks:team:%d:keys", teamID)
}

func (c *RedisCache) GetTasks(ctx context.Context, key string) ([]domain.Task, bool, error) {
	raw, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var tasks []domain.Task
	if err := json.Unmarshal(raw, &tasks); err != nil {
		return nil, false, err
	}
	return tasks, true, nil
}

// SetTasks кладёт срез в кеш с TTL и регистрирует ключ в set'е команды,
// чтобы потом можно было точечно инвалидировать все срезы этой команды.
func (c *RedisCache) SetTasks(ctx context.Context, key string, tasks []domain.Task) error {
	raw, err := json.Marshal(tasks)
	if err != nil {
		return err
	}
	teamID := teamIDFromKey(key)
	pipe := c.client.TxPipeline()
	pipe.Set(ctx, key, raw, c.ttl)
	if teamID > 0 {
		pipe.SAdd(ctx, teamSetKey(teamID), key)
		pipe.Expire(ctx, teamSetKey(teamID), c.ttl)
	}
	_, err = pipe.Exec(ctx)
	return err
}

// InvalidateTeam удаляет все закешированные срезы списка задач команды.
func (c *RedisCache) InvalidateTeam(ctx context.Context, teamID int64) error {
	setKey := teamSetKey(teamID)
	keys, err := c.client.SMembers(ctx, setKey).Result()
	if err != nil {
		return err
	}
	pipe := c.client.TxPipeline()
	if len(keys) > 0 {
		pipe.Del(ctx, keys...)
	}
	pipe.Del(ctx, setKey)
	_, err = pipe.Exec(ctx)
	return err
}

// teamIDFromKey достаёт teamID из ключа вида "tasks:team:<id>:...".
func teamIDFromKey(key string) int64 {
	const prefix = "tasks:team:"
	if !strings.HasPrefix(key, prefix) {
		return 0
	}
	rest := key[len(prefix):]
	end := strings.IndexByte(rest, ':')
	if end < 0 {
		return 0
	}
	id, err := strconv.ParseInt(rest[:end], 10, 64)
	if err != nil {
		return 0
	}
	return id
}
