package domain

import (
	"errors"
	"time"
)

type OrderStatus string

const (
	StatusDraft     OrderStatus = "draft"
	StatusConfirmed OrderStatus = "confirmed"
	StatusCompleted OrderStatus = "completed"
	StatusCancelled OrderStatus = "cancelled"
)

var ErrInvalidStateTransition = errors.New("invalid state transition: order must be confirmed to complete")

type Change struct {
	Field string
	Value interface{}
}

type ChangeTracker struct {
	changes []Change
}

func (ct *ChangeTracker) Track(field string, value interface{}) {
	ct.changes = append(ct.changes, Change{Field: field, Value: value})
}

func (ct *ChangeTracker) Changes() []Change {
	return ct.changes
}

type Order struct {
	id          string
	customerID  string
	totalAmount int64
	status      OrderStatus
	completedAt time.Time

	Changes ChangeTracker
	Events  EventRaiser
}

func NewOrder(id, customerID string, totalAmount int64, status OrderStatus) *Order {
	return &Order{
		id:          id,
		customerID:  customerID,
		totalAmount: totalAmount,
		status:      status,
	}
}

func (o *Order) ID() string             { return o.id }
func (o *Order) CustomerID() string     { return o.customerID }
func (o *Order) TotalAmount() int64     { return o.totalAmount }
func (o *Order) Status() OrderStatus    { return o.status }
func (o *Order) CompletedAt() time.Time { return o.completedAt }

func (o *Order) Complete(now time.Time) error {
	if o.status != StatusConfirmed {
		return ErrInvalidStateTransition
	}

	o.status = StatusCompleted
	o.completedAt = now

	o.Changes.Track("status", string(StatusCompleted))
	o.Changes.Track("completed_at", now)

	o.Events.Raise(OrderCompletedEvent{
		OrderID:     o.id,
		CustomerID:  o.customerID,
		TotalAmount: o.totalAmount,
		CompletedAt: now,
	})

	return nil
}
