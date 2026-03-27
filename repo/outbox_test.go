package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/giorgim/senior-task-1/contracts"
	"github.com/giorgim/senior-task-1/domain"
	"github.com/giorgim/senior-task-1/repo"
)

// --- Test fakes ---

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

type fakeOutboxStore struct {
	pending          []repo.OutboxEntry
	processed        map[string]time.Time
	failed           map[string]string
	inserted         []repo.OutboxEntry
	markProcessedErr map[string]error
}

func newFakeOutboxStore(entries []repo.OutboxEntry) *fakeOutboxStore {
	return &fakeOutboxStore{
		pending:   entries,
		processed: make(map[string]time.Time),
		failed:    make(map[string]string),
	}
}

func (s *fakeOutboxStore) InsertMut(entry repo.OutboxEntry) *contracts.Mutation {
	s.inserted = append(s.inserted, entry)
	return &contracts.Mutation{
		Name:  "insert_outbox",
		Apply: func(ctx context.Context) error { return nil },
	}
}

func (s *fakeOutboxStore) ReadPending(_ context.Context, limit int) ([]repo.OutboxEntry, error) {
	if limit > len(s.pending) {
		limit = len(s.pending)
	}
	return s.pending[:limit], nil
}

func (s *fakeOutboxStore) MarkProcessed(_ context.Context, id string, processedAt time.Time) error {
	if s.markProcessedErr != nil {
		if err, ok := s.markProcessedErr[id]; ok {
			return err
		}
	}
	s.processed[id] = processedAt
	return nil
}

func (s *fakeOutboxStore) MarkFailed(_ context.Context, id string, errMsg string) error {
	s.failed[id] = errMsg
	return nil
}

type fakePublisher struct {
	published []string
	failIDs   map[string]bool
}

func (p *fakePublisher) Publish(_ context.Context, entry repo.OutboxEntry) error {
	if p.failIDs[entry.ID] {
		return errors.New("publish failed")
	}
	p.published = append(p.published, entry.ID)
	return nil
}

// --- Worker tests ---

func TestWorker_ProcessPending_Success(t *testing.T) {
	entries := []repo.OutboxEntry{
		{ID: "evt-1", EventType: "OrderCompleted"},
		{ID: "evt-2", EventType: "OrderCompleted"},
	}
	store := newFakeOutboxStore(entries)
	publisher := &fakePublisher{failIDs: map[string]bool{}}
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	worker := repo.NewOutboxWorker(store, publisher, &fakeClock{now: now})

	processed, err := worker.ProcessPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if processed != 2 {
		t.Errorf("expected 2 processed, got %d", processed)
	}
	if len(store.processed) != 2 {
		t.Errorf("expected 2 entries marked processed, got %d", len(store.processed))
	}
	if len(publisher.published) != 2 {
		t.Errorf("expected 2 published, got %d", len(publisher.published))
	}
}

func TestWorker_PublishFailure_EntryRemainsPending(t *testing.T) {
	entries := []repo.OutboxEntry{
		{ID: "evt-1", EventType: "OrderCompleted"},
		{ID: "evt-2", EventType: "OrderCompleted"},
	}
	store := newFakeOutboxStore(entries)
	publisher := &fakePublisher{failIDs: map[string]bool{"evt-1": true}}
	worker := repo.NewOutboxWorker(store, publisher, &fakeClock{now: time.Now()})

	processed, err := worker.ProcessPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if processed != 1 {
		t.Errorf("expected 1 processed, got %d", processed)
	}
	if _, ok := store.processed["evt-1"]; ok {
		t.Error("evt-1 should NOT be marked processed after publish failure")
	}
	if _, ok := store.processed["evt-2"]; !ok {
		t.Error("evt-2 should be marked processed")
	}
	if _, ok := store.failed["evt-1"]; !ok {
		t.Error("evt-1 should be marked failed for later retry")
	}
}

func TestWorker_MarkProcessedFails_AtLeastOnce(t *testing.T) {
	entries := []repo.OutboxEntry{
		{ID: "evt-1", EventType: "OrderCompleted"},
	}
	store := newFakeOutboxStore(entries)
	store.markProcessedErr = map[string]error{
		"evt-1": errors.New("database write failed"),
	}
	publisher := &fakePublisher{failIDs: map[string]bool{}}
	worker := repo.NewOutboxWorker(store, publisher, &fakeClock{now: time.Now()})

	processed, err := worker.ProcessPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(publisher.published) != 1 {
		t.Fatalf("expected 1 published, got %d", len(publisher.published))
	}

	if processed != 0 {
		t.Errorf("expected 0 processed (mark failed), got %d", processed)
	}
	if _, ok := store.processed["evt-1"]; ok {
		t.Error("evt-1 must NOT be marked processed when MarkProcessed fails")
	}
}

// --- Repo-level tests ---

func TestOutboxMuts_DrainsEvents(t *testing.T) {
	store := newFakeOutboxStore(nil)
	orderRepo := repo.NewOrderRepo(store)

	order := domain.NewOrder("order-1", "customer-1", 5000, domain.StatusConfirmed)
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	if err := order.Complete(now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	muts1 := orderRepo.OutboxMuts(order)
	if len(muts1) != 1 {
		t.Fatalf("first call: expected 1 mutation, got %d", len(muts1))
	}

	muts2 := orderRepo.OutboxMuts(order)
	if len(muts2) != 0 {
		t.Errorf("second call: expected 0 mutations (events drained), got %d", len(muts2))
	}

	if len(store.inserted) != 1 {
		t.Errorf("expected 1 inserted entry total, got %d", len(store.inserted))
	}
}

func TestOutboxMuts_UsesEventTime(t *testing.T) {
	store := newFakeOutboxStore(nil)
	orderRepo := repo.NewOrderRepo(store)

	businessTime := time.Date(2020, 6, 15, 9, 30, 0, 0, time.UTC)
	order := domain.NewOrder("order-1", "customer-1", 5000, domain.StatusConfirmed)
	if err := order.Complete(businessTime); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_ = orderRepo.OutboxMuts(order)

	if len(store.inserted) != 1 {
		t.Fatalf("expected 1 inserted entry, got %d", len(store.inserted))
	}

	entry := store.inserted[0]

	if !entry.OccurredAt.Equal(businessTime) {
		t.Errorf("OccurredAt: expected %v (business time), got %v", businessTime, entry.OccurredAt)
	}
	if !entry.CreatedAt.Equal(businessTime) {
		t.Errorf("CreatedAt: expected %v (event time), got %v", businessTime, entry.CreatedAt)
	}
	if time.Since(entry.OccurredAt) < 365*24*time.Hour {
		t.Error("OccurredAt suspiciously close to wall clock — may be using time.Now()")
	}
}
