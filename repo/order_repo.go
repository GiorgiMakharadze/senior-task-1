package repo

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/giorgim/senior-task-1/contracts"
	"github.com/giorgim/senior-task-1/domain"
)

type orderRow struct {
	ID          string
	CustomerID  string
	TotalAmount int64
	Status      string
	CompletedAt time.Time
}

type OrderRepo struct {
	mu     sync.RWMutex
	orders map[string]*orderRow
	outbox OutboxStore
}

func NewOrderRepo(outbox OutboxStore) *OrderRepo {
	return &OrderRepo{
		orders: make(map[string]*orderRow),
		outbox: outbox,
	}
}

func (r *OrderRepo) Save(order *domain.Order) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.orders[order.ID()] = &orderRow{
		ID:          order.ID(),
		CustomerID:  order.CustomerID(),
		TotalAmount: order.TotalAmount(),
		Status:      string(order.Status()),
		CompletedAt: order.CompletedAt(),
	}
}

func (r *OrderRepo) Retrieve(_ context.Context, id string) (*domain.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	row, ok := r.orders[id]
	if !ok {
		return nil, fmt.Errorf("order not found: %s", id)
	}
	return domain.ReconstructOrder(row.ID, row.CustomerID, row.TotalAmount, domain.OrderStatus(row.Status), row.CompletedAt), nil
}

func (r *OrderRepo) UpdateMut(order *domain.Order) *contracts.Mutation {
	return &contracts.Mutation{
		Name: "update_order",
		Apply: func(ctx context.Context) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			row, ok := r.orders[order.ID()]
			if !ok {
				return fmt.Errorf("order not found for update: %s", order.ID())
			}
			for _, c := range order.Changes.Changes() {
				switch c.Field {
				case "status":
					v, ok := c.Value.(string)
					if !ok {
						return fmt.Errorf("update order %s: status change value is %T, expected string", order.ID(), c.Value)
					}
					row.Status = v
				case "completed_at":
					v, ok := c.Value.(time.Time)
					if !ok {
						return fmt.Errorf("update order %s: completed_at change value is %T, expected time.Time", order.ID(), c.Value)
					}
					row.CompletedAt = v
				}
			}
			return nil
		},
	}
}

func (r *OrderRepo) OutboxMuts(order *domain.Order) []*contracts.Mutation {
	var muts []*contracts.Mutation
	for _, event := range order.Events.Drain() {
		entry := OutboxEntry{
			ID:            newUUID(),
			AggregateType: "Order",
			AggregateID:   order.ID(),
			EventType:     event.EventType(),
			Payload:       toJSON(event),
			OccurredAt:    event.OccurredAt(),
			CreatedAt:     event.OccurredAt(),
		}
		muts = append(muts, r.outbox.InsertMut(entry))
	}
	return muts
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func toJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}
