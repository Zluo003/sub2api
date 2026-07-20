package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestYingzoDownloadTicketStoreSupportsHeadRangeAndRetryUntilExpiry(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	store := NewYingzoDownloadTicketStore(client)
	payload := []byte(`{"release_id":"release-1"}`)
	require.NoError(t, store.Store(context.Background(), "ticket-1", payload, time.Minute))

	consumed, err := store.Get(context.Background(), "ticket-1")
	require.NoError(t, err)
	require.Equal(t, payload, consumed)

	retried, err := store.Get(context.Background(), "ticket-1")
	require.NoError(t, err)
	require.Equal(t, payload, retried)

	server.FastForward(time.Minute)
	_, err = store.Get(context.Background(), "ticket-1")
	require.True(t, errors.Is(err, service.ErrYingzoDownloadTicketNotFound))
}
