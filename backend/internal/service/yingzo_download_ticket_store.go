package service

import (
	"context"
	"errors"
	"time"
)

var ErrYingzoDownloadTicketNotFound = errors.New("yingzo download ticket not found")

type YingzoDownloadTicketStore interface {
	Store(ctx context.Context, ticket string, payload []byte, ttl time.Duration) error
	Get(ctx context.Context, ticket string) ([]byte, error)
}
