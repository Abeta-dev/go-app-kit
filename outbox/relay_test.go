package outbox_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/abeta-dev/go-app-kit/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestBackoffWithJitter_HighAttemptsAndMaxInt64Overflow(t *testing.T) {
	base := 100 * time.Millisecond
	max := 5 * time.Second

	for _, attempt := range []int{31, 40, 63, 100} {
		d := outbox.BackoffWithJitter(attempt, base, max)
		assert.GreaterOrEqual(t, d, time.Duration(0))
		assert.LessOrEqual(t, d, max)
	}

	// Trigger multiplier > math.MaxInt64/int64(baseDelay) and ceiling == math.MaxInt64 (with maxDelay = 0)
	dOverflow := outbox.BackoffWithJitter(30, 10*time.Second, 0)
	assert.GreaterOrEqual(t, dOverflow, time.Duration(0))
}

func TestPGStore_InsertAndMark_ZeroTimesAndDBErrors(t *testing.T) {
	execCalls := 0
	mockOp := &mockDBOperator{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			execCalls++
			if execCalls > 1 {
				return pgconn.CommandTag{}, errors.New("simulated db exec error")
			}
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}

	store := outbox.NewPGStore(mockOp)
	// 1. Insert with zero ScheduledAt and NextRetryAt (and op == nil)
	err := store.Insert(context.Background(), nil, outbox.Event{
		ID:            uuid.New(),
		AggregateType: "Order",
		AggregateID:   "ORD-ZERO",
		EventType:     "OrderCreated",
		Payload:       []byte(`{}`),
		Status:        outbox.StatusPending,
	})
	require.NoError(t, err)

	// 2. MarkPublished DB error
	err = store.MarkPublished(context.Background(), uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to mark outbox event as published")

	// 3. MarkFailed DB error
	err = store.MarkFailed(context.Background(), uuid.New(), "err", time.Now(), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update failed outbox event")
}

type nilLeaseStore struct {
	events       []outbox.Event
	publishedIDs []uuid.UUID
	failedIDs    []uuid.UUID
}

func (n *nilLeaseStore) Insert(ctx context.Context, op outbox.DBOperator, event outbox.Event) error {
	return nil
}

func (n *nilLeaseStore) FetchPendingBatch(ctx context.Context, limit int) ([]outbox.Event, error) {
	return n.events, nil
}

func (n *nilLeaseStore) MarkPublished(ctx context.Context, id uuid.UUID, leaseToken ...uuid.UUID) error {
	n.publishedIDs = append(n.publishedIDs, id)
	return nil
}

func (n *nilLeaseStore) MarkFailed(ctx context.Context, id uuid.UUID, lastErr string, nextRetry time.Time, finalFail bool, leaseToken ...uuid.UUID) error {
	n.failedIDs = append(n.failedIDs, id)
	return nil
}

func TestRelay_ProcessBatch_NilLeaseToken(t *testing.T) {
	evt1, _ := outbox.NewEvent("Order", "ORD-NIL-1", "Created", map[string]string{"a": "1"})
	evt1.LeaseToken = nil
	evt2, _ := outbox.NewEvent("Order", "ORD-NIL-2", "Created", map[string]string{"a": "2"})
	evt2.LeaseToken = nil

	store := &nilLeaseStore{events: []outbox.Event{*evt1, *evt2}}
	publisher := outbox.PublisherFunc(func(ctx context.Context, event outbox.Event) error {
		if event.ID == evt2.ID {
			return errors.New("publish error on nil lease event")
		}
		return nil
	})

	relay, err := outbox.NewRelay(outbox.RelayConfig{
		Store:     store,
		Publisher: publisher,
	})
	require.NoError(t, err)

	count, err := relay.ProcessBatch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Equal(t, []uuid.UUID{evt1.ID}, store.publishedIDs)
	assert.Equal(t, []uuid.UUID{evt2.ID}, store.failedIDs)

	// Also verify context cancellation mid-batch between events
	batchCtx, batchCancel := context.WithCancel(context.Background())
	midCancelPublisher := outbox.PublisherFunc(func(ctx context.Context, event outbox.Event) error {
		batchCancel()
		return nil
	})
	relayCancel, err := outbox.NewRelay(outbox.RelayConfig{
		Store:     &nilLeaseStore{events: []outbox.Event{*evt1, *evt2}},
		Publisher: midCancelPublisher,
	})
	require.NoError(t, err)
	partialCount, err := relayCancel.ProcessBatch(batchCtx)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, partialCount)
}

type cancelOnDrainStore struct {
	cancel context.CancelFunc
	calls  int
}

func (c *cancelOnDrainStore) Insert(ctx context.Context, op outbox.DBOperator, event outbox.Event) error {
	return nil
}

func (c *cancelOnDrainStore) FetchPendingBatch(ctx context.Context, limit int) ([]outbox.Event, error) {
	c.calls++
	evts := make([]outbox.Event, limit)
	for i := 0; i < limit; i++ {
		evts[i] = outbox.Event{ID: uuid.New(), EventType: "DrainCancel"}
	}
	return evts, nil
}

func (c *cancelOnDrainStore) MarkPublished(ctx context.Context, id uuid.UUID, leaseToken ...uuid.UUID) error {
	c.cancel()
	return nil
}

func (c *cancelOnDrainStore) MarkFailed(ctx context.Context, id uuid.UUID, lastErr string, nextRetry time.Time, finalFail bool, leaseToken ...uuid.UUID) error {
	return nil
}

func TestRelay_RunWorker_DrainContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := &cancelOnDrainStore{cancel: cancel}
	publisher := outbox.PublisherFunc(func(ctx context.Context, event outbox.Event) error {
		return nil
	})

	relay, err := outbox.NewRelay(outbox.RelayConfig{
		BatchSize:    1,
		PollInterval: 5 * time.Millisecond,
		Store:        store,
		Publisher:    publisher,
	})
	require.NoError(t, err)

	err = relay.Start(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}
