package outbox_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umesh0492/go-app-kit/outbox"
)

func TestRelay_ProcessBatch_PanickingPublisher(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()

	evt, err := outbox.NewEvent("Order", "ORD-123", "OrderCreated", map[string]string{"user": "alice"})
	require.NoError(t, err)
	err = store.Insert(ctx, nil, *evt)
	require.NoError(t, err)

	panickingPublisher := outbox.PublisherFunc(func(ctx context.Context, event outbox.Event) error {
		panic("nil pointer dereference in kafka producer")
	})

	relay, err := outbox.NewRelay(outbox.RelayConfig{
		Store:          store,
		Publisher:      panickingPublisher,
		BatchSize:      10,
		PollInterval:   50 * time.Millisecond,
		PublishTimeout: 2 * time.Second,
		LeaseDuration:  10 * time.Second,
	})
	require.NoError(t, err)

	// ProcessBatch must not panic and must capture publisher panic as error
	count, err := relay.ProcessBatch(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "event should be counted as processed despite publisher panic")

	// Verify the event was marked as failed in the store
	store.mu.Lock()
	failedInfo, ok := store.failedEvents[evt.ID]
	store.mu.Unlock()

	require.True(t, ok, "event must be marked as failed in store")
	assert.Contains(t, failedInfo.err, "publisher panic: nil pointer dereference in kafka producer")
}

type stallTestStore struct {
	*mockStore
	mu              sync.Mutex
	failForEventID  uuid.UUID
	markFailedCalls []uuid.UUID
}

func (s *stallTestStore) MarkFailed(ctx context.Context, id uuid.UUID, lastErr string, nextRetry time.Time, finalFail bool, leaseToken ...uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markFailedCalls = append(s.markFailedCalls, id)
	if id == s.failForEventID {
		return errors.New("db error: failed to record failure state")
	}
	return s.mockStore.MarkFailed(ctx, id, lastErr, nextRetry, finalFail, leaseToken...)
}

func TestRelay_ProcessBatch_MarkFailedError_DoesNotStallBatch(t *testing.T) {
	ctx := context.Background()
	baseStore := newMockStore()

	evt1, err := outbox.NewEvent("Order", "ORD-1", "OrderCreated", map[string]string{"id": "1"})
	require.NoError(t, err)
	evt2, err := outbox.NewEvent("Order", "ORD-2", "OrderCreated", map[string]string{"id": "2"})
	require.NoError(t, err)

	require.NoError(t, baseStore.Insert(ctx, nil, *evt1))
	require.NoError(t, baseStore.Insert(ctx, nil, *evt2))

	store := &stallTestStore{
		mockStore:      baseStore,
		failForEventID: evt1.ID,
	}

	publisher := outbox.PublisherFunc(func(ctx context.Context, event outbox.Event) error {
		if event.ID == evt1.ID {
			return errors.New("network transient error")
		}
		return nil
	})

	relay, err := outbox.NewRelay(outbox.RelayConfig{
		Store:          store,
		Publisher:      publisher,
		BatchSize:      10,
		PollInterval:   50 * time.Millisecond,
		PublishTimeout: 2 * time.Second,
		LeaseDuration:  10 * time.Second,
	})
	require.NoError(t, err)

	// Processing batch must not abort on evt1 MarkFailed error, and must continue to evt2
	count, err := relay.ProcessBatch(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error: failed to record failure state")
	assert.Equal(t, 1, count, "evt2 was published successfully, evt1 skipped after MarkFailed error")

	// Verify evt2 was published
	baseStore.mu.Lock()
	assert.Contains(t, baseStore.publishedIDs, evt2.ID)
	baseStore.mu.Unlock()

	// Verify MarkFailed was attempted on evt1
	store.mu.Lock()
	assert.Contains(t, store.markFailedCalls, evt1.ID)
	store.mu.Unlock()
}

type panickingStore struct {
	*mockStore
	panicOnFetch bool
}

func (p *panickingStore) FetchPendingBatch(ctx context.Context, limit int) ([]outbox.Event, error) {
	if p.panicOnFetch {
		panic("unexpected database driver panic")
	}
	return p.mockStore.FetchPendingBatch(ctx, limit)
}

func TestRelay_RunWorker_RecoversFromPanic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	store := &panickingStore{
		mockStore:    newMockStore(),
		panicOnFetch: true,
	}

	dummyPublisher := outbox.PublisherFunc(func(ctx context.Context, event outbox.Event) error {
		return nil
	})

	relay, err := outbox.NewRelay(outbox.RelayConfig{
		Store:          store,
		Publisher:      dummyPublisher,
		BatchSize:      5,
		PollInterval:   20 * time.Millisecond,
		PublishTimeout: 1 * time.Second,
		LeaseDuration:  5 * time.Second,
		Concurrency:    2,
	})
	require.NoError(t, err)

	// Start relay - worker goroutine will panic on FetchPendingBatch,
	// but the defer-recover ensures the process/test does not crash.
	err = relay.Start(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
