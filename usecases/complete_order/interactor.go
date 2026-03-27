package complete_order

import (
	"context"

	"github.com/giorgim/senior-task-1/contracts"
)

type Request struct {
	OrderID string
}

type Interactor struct {
	repo  contracts.OrderRepository
	clock contracts.Clock
}

func NewInteractor(repo contracts.OrderRepository, clock contracts.Clock) *Interactor {
	return &Interactor{repo: repo, clock: clock}
}

func (uc *Interactor) Execute(ctx context.Context, req *Request) (*contracts.Plan, error) {
	order, err := uc.repo.Retrieve(ctx, req.OrderID)
	if err != nil {
		return nil, err
	}

	now := uc.clock.Now()
	if err := order.Complete(now); err != nil {
		return nil, err
	}

	plan := &contracts.Plan{}
	plan.Mutations = append(plan.Mutations, uc.repo.UpdateMut(order))
	plan.Mutations = append(plan.Mutations, uc.repo.OutboxMuts(order)...)

	return plan, nil
}
