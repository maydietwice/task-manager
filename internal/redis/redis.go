package rdb

import (
	"context"
	"log"
	"strconv"

	"github.com/redis/go-redis/v9"
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
	return r.rdb.HSet(ctx, key, fields).Err()
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
