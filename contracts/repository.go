package contracts

import (
	"context"
	"time"

	"github.com/giorgim/senior-task-1/domain"
)

type Mutation struct {
	Name  string
	Apply func(ctx context.Context) error
}

type Plan struct {
	Mutations []*Mutation
}

type CommitFunc func(ctx context.Context, mutations []*Mutation) error

func (p *Plan) Execute(ctx context.Context, commit CommitFunc) error {
	return commit(ctx, p.Mutations)
}

type Clock interface {
	Now() time.Time
}

type OrderRepository interface {
	Retrieve(ctx context.Context, id string) (*domain.Order, error)
	UpdateMut(order *domain.Order) *Mutation
	OutboxMuts(order *domain.Order) []*Mutation
}
