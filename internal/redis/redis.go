package rdb

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	tasksCacheExp   = time.Second * 30
	userChatInfoExp = time.Hour * 2
)

type Repository struct {
	rdb *redis.Client
}

func NewConnection(pwd, addr string) (*Repository, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: pwd,
		DB:       0,
		Protocol: 2,
	})

	ctx := context.Background()

	err := rdb.Ping(ctx).Err()
	if err != nil {
		return nil, err
	}

	return &Repository{rdb: rdb}, nil
}

func (r *Repository) CloseConnection() {
	err := r.rdb.Close()
	if err != nil {
		log.Printf("redis connection closing error: %v", err)
	}
}

func (r *Repository) SetChatInfo(ctx context.Context, chatId int64, fields ...string) error {
	key := "state:" + strconv.Itoa(int(chatId))
	err := r.rdb.HSet(ctx, key, fields).Err()
	if err != nil {
		return err
	}
	r.rdb.Expire(ctx, key, userChatInfoExp)

	return nil
}

func (r *Repository) GetChatInfo(ctx context.Context, chatId int64) (map[string]string, error) {
	info, err := r.rdb.HGetAll(ctx, "state:"+strconv.Itoa(int(chatId))).Result()

	return info, err
}

func (r *Repository) ClearState(ctx context.Context, chatId int64) {
	_, err := r.rdb.Del(ctx, "state:"+strconv.Itoa(int(chatId))).Result()
	if err != nil {
		log.Printf("clear state err | chatId: %v, err: %v", chatId, err)
	}
}

func (r *Repository) CacheUserTasks(ctx context.Context, chatId int64, page int, tasks string) error {
	key := fmt.Sprintf("list:%d:%d", int(chatId), page)
	return r.rdb.Set(ctx, key, tasks, tasksCacheExp).Err()
}

func (r *Repository) GetCachedUserTasks(ctx context.Context, chatId int64, page int) []byte {
	key := fmt.Sprintf("list:%d:%d", int(chatId), page)
	tasks, err := r.rdb.Get(ctx, key).Result()
	if err != nil {
		log.Printf("Error getting user tasks: %v", err)

		return nil
	}

	return []byte(tasks)
}

func (r *Repository) ClearCache(ctx context.Context, chatId int64) error {
	pattern := fmt.Sprintf("list:%d:*", int(chatId))
	var cursor uint64
	for {
		keys, nextCursor, err := r.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}

		if len(keys) > 0 {
			if _, err := r.rdb.Unlink(ctx, keys...).Result(); err != nil {
				return err
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}
