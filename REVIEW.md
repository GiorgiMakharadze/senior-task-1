# Code Review: Buggy Implementation

## The Buggy Code

```go
func (uc *Interactor) Execute(ctx context.Context, req *Request) error {
    order, _ := uc.repo.Retrieve(ctx, req.OrderID)

    order.status = StatusCompleted
    order.completedAt = time.Now()

    if err := uc.db.Update(order); err != nil {
        return err
    }

    event := OrderCompletedEvent{
        OrderID:    order.id,
        CustomerID: order.customerID,
        Total:      order.totalAmount,
    }

    if err := uc.eventBus.Publish(event); err != nil {
        return err 
    }

    return nil
}
```

## 1. Why Direct Publish Breaks Reliability

The database update and the event publish are two independent operations with no shared transactional guarantee. Either can succeed while the other fails. This creates a window where the system's persisted state and its downstream event delivery diverge — a state that may never self-correct.

Additional issues in this code:

- The `Retrieve` error is silently ignored (`order, _ :=`), risking a nil pointer panic.
- State mutation bypasses the aggregate's domain method, skipping business rule validation.
- `time.Now()` is called directly, making the operation non-deterministic and untestable.
- The event is constructed outside the domain, decoupling business facts from the aggregate that owns them.

## 2. The Exact Failure Scenario

1. `uc.db.Update(order)` succeeds — the order is now `StatusCompleted` in the database.
2. `uc.eventBus.Publish(event)` fails — network timeout, broker unavailable, process crash, etc.
3. The function returns an error to the caller.

**Result:** The order is permanently marked as completed in the database, but the `OrderCompletedEvent` was never delivered. Downstream systems (billing, fulfillment, notifications) never learn about the completion. There is no mechanism to detect or recover from this inconsistency, because the intent to publish was never durably recorded.

The reverse is also possible depending on implementation: if the publish succeeds but the DB update fails or the process crashes before commit, downstream systems receive an event about a completion that never actually happened.

## 3. Why Outbox Must Be in the Same Transaction

The outbox pattern solves this by recording the intent to publish as a row in an outbox table, written in the **same database transaction** as the aggregate update.

- If the transaction commits, both the order update **and** the outbox entry are durable. A background worker will eventually read and publish the event.
- If the transaction rolls back, neither the order update nor the outbox entry persists. No phantom event can be published.

This eliminates the failure window between "state changed" and "event published." The outbox entry is a durable record of intent — the worker converts it into actual delivery later, with at-least-once semantics.

If the outbox insert were in a **separate** transaction from the order update, the same two-operation divergence problem would return: one could commit without the other.
