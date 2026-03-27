package complete_order_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/giorgim/senior-task-1/contracts"
	"github.com/giorgim/senior-task-1/domain"
	complete_order "github.com/giorgim/senior-task-1/usecases/complete_order"
)

// --- Test fakes ---

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

type fakeRepo struct {
	order     *domain.Order
	err       error
	retrieved bool
}

func (r *fakeRepo) Retrieve(_ context.Context, _ string) (*domain.Order, error) {
	r.retrieved = true
	if r.err != nil {
		return nil, r.err
	}
	return r.order, nil
}

func (r *fakeRepo) UpdateMut(order *domain.Order) *contracts.Mutation {
	return &contracts.Mutation{
		Name:  "update_order",
		Apply: func(ctx context.Context) error { return nil },
	}
}

// Note: this fake reads Events() without draining. The real OrderRepo uses
// Drain() to prevent duplicate outbox inserts — that behavior is tested
// separately at the repo level in repo/outbox_test.go.
// Примечание: этот фейк читает Events() без их очистки (drain).
// В реальном OrderRepo используется   Drain(), чтобы предотвратить повторные вставки в outbox
// это поведение тестируется отдельно на уровне репозитория в repo/outbox_test.go.
func (r *fakeRepo) OutboxMuts(order *domain.Order) []*contracts.Mutation {
	var muts []*contracts.Mutation
	for _, e := range order.Events.Events() {
		muts = append(muts, &contracts.Mutation{
			Name:  "insert_outbox_" + e.EventType(),
			Apply: func(ctx context.Context) error { return nil },
		})
	}
	return muts
}

// --- Tests ---

func TestExecute_Success(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	order := domain.NewOrder("order-1", "customer-1", 5000, domain.StatusConfirmed)

	repo := &fakeRepo{order: order}
	clock := &fakeClock{now: now}
	uc := complete_order.NewInteractor(repo, clock)

	plan, err := uc.Execute(context.Background(), &complete_order.Request{OrderID: "order-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Mutations) != 2 {
		t.Fatalf("expected 2 mutations, got %d", len(plan.Mutations))
	}
	if plan.Mutations[0].Name != "update_order" {
		t.Errorf("first mutation: expected update_order, got %s", plan.Mutations[0].Name)
	}
	if plan.Mutations[1].Name != "insert_outbox_OrderCompleted" {
		t.Errorf("second mutation: expected insert_outbox_OrderCompleted, got %s", plan.Mutations[1].Name)
	}

	if order.Status() != domain.StatusCompleted {
		t.Errorf("expected status completed, got %s", order.Status())
	}
	if !order.CompletedAt().Equal(now) {
		t.Errorf("expected completedAt %v, got %v", now, order.CompletedAt())
	}
}

func TestExecute_EventContent(t *testing.T) {
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	order := domain.NewOrder("order-42", "cust-7", 12399, domain.StatusConfirmed)

	repo := &fakeRepo{order: order}
	clock := &fakeClock{now: now}
	uc := complete_order.NewInteractor(repo, clock)

	_, err := uc.Execute(context.Background(), &complete_order.Request{OrderID: "order-42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := order.Events.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	evt, ok := events[0].(domain.OrderCompletedEvent)
	if !ok {
		t.Fatal("expected OrderCompletedEvent")
	}
	if evt.OrderID != "order-42" {
		t.Errorf("event OrderID: expected order-42, got %s", evt.OrderID)
	}
	if evt.CustomerID != "cust-7" {
		t.Errorf("event CustomerID: expected cust-7, got %s", evt.CustomerID)
	}
	if evt.TotalAmount != 12399 {
		t.Errorf("event TotalAmount: expected 12399, got %d", evt.TotalAmount)
	}
	if !evt.CompletedAt.Equal(now) {
		t.Errorf("event CompletedAt: expected %v, got %v (must come from clock, not time.Now())", now, evt.CompletedAt)
	}
}

func TestExecute_InvalidState_Draft(t *testing.T) {
	order := domain.NewOrder("order-1", "customer-1", 5000, domain.StatusDraft)

	repo := &fakeRepo{order: order}
	clock := &fakeClock{now: time.Now()}
	uc := complete_order.NewInteractor(repo, clock)

	plan, err := uc.Execute(context.Background(), &complete_order.Request{OrderID: "order-1"})
	if err == nil {
		t.Fatal("expected error for draft order, got nil")
	}
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("expected ErrInvalidStateTransition, got %v", err)
	}
	if plan != nil {
		t.Errorf("expected nil plan on invalid state, got %+v", plan)
	}

	if len(order.Events.Events()) != 0 {
		t.Errorf("expected 0 events on failed completion, got %d", len(order.Events.Events()))
	}
}

func TestExecute_InvalidState_AlreadyCompleted(t *testing.T) {
	order := domain.NewOrder("order-1", "customer-1", 5000, domain.StatusCompleted)

	repo := &fakeRepo{order: order}
	clock := &fakeClock{now: time.Now()}
	uc := complete_order.NewInteractor(repo, clock)

	_, err := uc.Execute(context.Background(), &complete_order.Request{OrderID: "order-1"})
	if err == nil {
		t.Fatal("expected error for already-completed order")
	}
	if !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Errorf("expected ErrInvalidStateTransition, got %v", err)
	}
}

func TestExecute_RetrieveError(t *testing.T) {
	repo := &fakeRepo{err: errors.New("database unavailable")}
	clock := &fakeClock{now: time.Now()}
	uc := complete_order.NewInteractor(repo, clock)

	plan, err := uc.Execute(context.Background(), &complete_order.Request{OrderID: "order-1"})
	if err == nil {
		t.Fatal("expected error from retrieve, got nil")
	}
	if plan != nil {
		t.Errorf("expected nil plan on retrieve error, got %+v", plan)
	}
}

func TestExecute_ChangesTracked(t *testing.T) {
	now := time.Date(2024, 3, 10, 8, 0, 0, 0, time.UTC)
	order := domain.NewOrder("order-1", "customer-1", 1000, domain.StatusConfirmed)

	repo := &fakeRepo{order: order}
	clock := &fakeClock{now: now}
	uc := complete_order.NewInteractor(repo, clock)

	_, err := uc.Execute(context.Background(), &complete_order.Request{OrderID: "order-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	changes := order.Changes.Changes()
	if len(changes) != 2 {
		t.Fatalf("expected 2 tracked changes, got %d", len(changes))
	}

	statusFound, timeFound := false, false
	for _, c := range changes {
		switch c.Field {
		case "status":
			if c.Value != string(domain.StatusCompleted) {
				t.Errorf("status change value: expected completed, got %v", c.Value)
			}
			statusFound = true
		case "completed_at":
			if ts, ok := c.Value.(time.Time); !ok || !ts.Equal(now) {
				t.Errorf("completed_at change value: expected %v, got %v", now, c.Value)
			}
			timeFound = true
		}
	}
	if !statusFound {
		t.Error("missing tracked change for status")
	}
	if !timeFound {
		t.Error("missing tracked change for completed_at")
	}
}

func TestPlan_SingleTransactionBoundary(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	order := domain.NewOrder("order-1", "customer-1", 5000, domain.StatusConfirmed)

	repo := &fakeRepo{order: order}
	clock := &fakeClock{now: now}
	uc := complete_order.NewInteractor(repo, clock)

	plan, err := uc.Execute(context.Background(), &complete_order.Request{OrderID: "order-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var commitCalls int
	var committedNames []string

	commit := contracts.CommitFunc(func(ctx context.Context, mutations []*contracts.Mutation) error {
		commitCalls++
		for _, m := range mutations {
			committedNames = append(committedNames, m.Name)
		}
		return nil
	})

	if err := plan.Execute(context.Background(), commit); err != nil {
		t.Fatalf("plan execute: %v", err)
	}

	if commitCalls != 1 {
		t.Errorf("expected 1 commit call (single transaction), got %d", commitCalls)
	}

	if len(committedNames) != 2 {
		t.Fatalf("expected 2 mutations in commit, got %d", len(committedNames))
	}
	if committedNames[0] != "update_order" {
		t.Errorf("first committed mutation: expected update_order, got %s", committedNames[0])
	}
	if committedNames[1] != "insert_outbox_OrderCompleted" {
		t.Errorf("second committed mutation: expected insert_outbox_OrderCompleted, got %s", committedNames[1])
	}
}
