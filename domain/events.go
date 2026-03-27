package domain

import "time"

type Event interface {
	EventType() string
	OccurredAt() time.Time
}

type OrderCompletedEvent struct {
	OrderID     string
	CustomerID  string
	TotalAmount int64
	CompletedAt time.Time
}

func (e OrderCompletedEvent) EventType() string     { return "OrderCompleted" }
func (e OrderCompletedEvent) OccurredAt() time.Time { return e.CompletedAt }
