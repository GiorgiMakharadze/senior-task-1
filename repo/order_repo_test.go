package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/giorgim/senior-task-1/domain"
	"github.com/giorgim/senior-task-1/repo"
)

func TestOrderRepo_RetrieveAndUpdate(t *testing.T) {
	store := newFakeOutboxStore(nil)
	r := repo.NewOrderRepo(store)

	order := domain.NewOrder("order-1", "cust-1", 9900, domain.StatusConfirmed)
	r.Save(order)

	retrieved, err := r.Retrieve(context.Background(), "order-1")
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if retrieved.Status() != domain.StatusConfirmed {
		t.Fatalf("expected confirmed, got %s", retrieved.Status())
	}

	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := retrieved.Complete(now); err != nil {
		t.Fatalf("complete: %v", err)
	}

	mut := r.UpdateMut(retrieved)
	if err := mut.Apply(context.Background()); err != nil {
		t.Fatalf("apply update: %v", err)
	}

	after, err := r.Retrieve(context.Background(), "order-1")
	if err != nil {
		t.Fatalf("re-retrieve: %v", err)
	}
	if after.Status() != domain.StatusCompleted {
		t.Errorf("expected completed, got %s", after.Status())
	}
	if after.TotalAmount() != 9900 {
		t.Errorf("totalAmount should be unchanged, got %d", after.TotalAmount())
	}
}

func TestOrderRepo_CompletedAtSurvivesRoundTrip(t *testing.T) {
	store := newFakeOutboxStore(nil)
	r := repo.NewOrderRepo(store)

	order := domain.NewOrder("order-rt", "cust-1", 5000, domain.StatusConfirmed)
	r.Save(order)

	retrieved, err := r.Retrieve(context.Background(), "order-rt")
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}

	completedAt := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)
	if err := retrieved.Complete(completedAt); err != nil {
		t.Fatalf("complete: %v", err)
	}

	if err := r.UpdateMut(retrieved).Apply(context.Background()); err != nil {
		t.Fatalf("apply update: %v", err)
	}

	after, err := r.Retrieve(context.Background(), "order-rt")
	if err != nil {
		t.Fatalf("re-retrieve: %v", err)
	}
	if after.Status() != domain.StatusCompleted {
		t.Errorf("expected completed, got %s", after.Status())
	}
	if !after.CompletedAt().Equal(completedAt) {
		t.Errorf("CompletedAt not preserved: expected %v, got %v", completedAt, after.CompletedAt())
	}
}

func TestOrderRepo_RetrieveNotFound(t *testing.T) {
	store := newFakeOutboxStore(nil)
	r := repo.NewOrderRepo(store)

	_, err := r.Retrieve(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing order")
	}
}
