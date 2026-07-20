package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const yingzoDownloadTicketPrefix = "yingzo:download:"

type YingzoDownloadTicketStore struct {
	client *redis.Client
}

func NewYingzoDownloadTicketStore(client *redis.Client) service.YingzoDownloadTicketStore {
	return &YingzoDownloadTicketStore{client: client}
}

func (s *YingzoDownloadTicketStore) Store(ctx context.Context, ticket string, payload []byte, ttl time.Duration) error {
	return s.client.Set(ctx, yingzoDownloadTicketPrefix+ticket, payload, ttl).Err()
}

func (s *YingzoDownloadTicketStore) Get(ctx context.Context, ticket string) ([]byte, error) {
	payload, err := s.client.Get(ctx, yingzoDownloadTicketPrefix+ticket).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, service.ErrYingzoDownloadTicketNotFound
	}
	return payload, err
}
