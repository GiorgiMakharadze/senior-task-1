package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/giorgim/senior-task-1/contracts"
)

type OutboxEntry struct {
	ID            string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       []byte
	OccurredAt    time.Time
	CreatedAt     time.Time
	ProcessedAt   *time.Time
	AttemptCount  int
	LastError     string
}

type OutboxStore interface {
	InsertMut(entry OutboxEntry) *contracts.Mutation
	ReadPending(ctx context.Context, limit int) ([]OutboxEntry, error)
	MarkProcessed(ctx context.Context, id string, processedAt time.Time) error
	MarkFailed(ctx context.Context, id string, errMsg string) error
}

type EventPublisher interface {
	Publish(ctx context.Context, entry OutboxEntry) error
}

type OutboxWorker struct {
	store     OutboxStore
	publisher EventPublisher
	clock     contracts.Clock
}

func NewOutboxWorker(store OutboxStore, publisher EventPublisher, clock contracts.Clock) *OutboxWorker {
	return &OutboxWorker{store: store, publisher: publisher, clock: clock}
}

func (w *OutboxWorker) ProcessPending(ctx context.Context, batchSize int) (int, error) {
	entries, err := w.store.ReadPending(ctx, batchSize)
	if err != nil {
		return 0, fmt.Errorf("read pending outbox entries: %w", err)
	}

	processed := 0
	for _, entry := range entries {
		if err := w.publisher.Publish(ctx, entry); err != nil {
			_ = w.store.MarkFailed(ctx, entry.ID, err.Error())
			continue
		}

		if err := w.store.MarkProcessed(ctx, entry.ID, w.clock.Now()); err != nil {
			continue
		}
		processed++
	}

	return processed, nil
}
