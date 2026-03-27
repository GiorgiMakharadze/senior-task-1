package repo

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"

	"github.com/giorgim/senior-task-1/contracts"
	"github.com/giorgim/senior-task-1/domain"
)

type OrderRepo struct {
	outbox OutboxStore
}

func NewOrderRepo(outbox OutboxStore) *OrderRepo {
	return &OrderRepo{outbox: outbox}
}

func (r *OrderRepo) Retrieve(ctx context.Context, id string) (*domain.Order, error) {
	return nil, fmt.Errorf("not implemented: retrieve order %s", id)
}

func (r *OrderRepo) UpdateMut(order *domain.Order) *contracts.Mutation {
	return &contracts.Mutation{
		Name: "update_order",
		Apply: func(ctx context.Context) error {
			_ = order.Changes.Changes()
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
