package domain

import (
	"testing"
	"time"
)

func TestComplete_Success(t *testing.T) {
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	order := NewOrder("order-1", "cust-1", 4999, StatusConfirmed)

	if err := order.Complete(now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if order.Status() != StatusCompleted {
		t.Errorf("expected completed, got %s", order.Status())
	}
	if !order.CompletedAt().Equal(now) {
		t.Errorf("completedAt: expected %v, got %v", now, order.CompletedAt())
	}

	events := order.Events.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	evt, ok := events[0].(OrderCompletedEvent)
	if !ok {
		t.Fatal("expected OrderCompletedEvent")
	}
	if evt.OrderID != "order-1" {
		t.Errorf("event OrderID: expected order-1, got %s", evt.OrderID)
	}
	if evt.CustomerID != "cust-1" {
		t.Errorf("event CustomerID: expected cust-1, got %s", evt.CustomerID)
	}
	if evt.TotalAmount != 4999 {
		t.Errorf("event TotalAmount: expected 4999, got %d", evt.TotalAmount)
	}
	if !evt.CompletedAt.Equal(now) {
		t.Errorf("event CompletedAt: expected %v, got %v", now, evt.CompletedAt)
	}
}

func TestComplete_InvalidTransitions(t *testing.T) {
	cases := []struct {
		name   string
		status OrderStatus
	}{
		{"draft", StatusDraft},
		{"already completed", StatusCompleted},
		{"cancelled", StatusCancelled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			order := NewOrder("order-1", "cust-1", 1000, tc.status)
			err := order.Complete(time.Now())
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err != ErrInvalidStateTransition {
				t.Errorf("expected ErrInvalidStateTransition, got %v", err)
			}
			if len(order.Events.Events()) != 0 {
				t.Error("no events should be raised on failed transition")
			}
			if len(order.Changes.Changes()) != 0 {
				t.Error("no changes should be tracked on failed transition")
			}
		})
	}
}

func TestComplete_ReconstructedOrderDoesNotRaiseEvents(t *testing.T) {
	completed := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	order := ReconstructOrder("order-1", "cust-1", 5000, StatusCompleted, completed)

	if len(order.Events.Events()) != 0 {
		t.Error("reconstitution must not raise events")
	}
	if len(order.Changes.Changes()) != 0 {
		t.Error("reconstitution must not track changes")
	}
}
