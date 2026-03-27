# Answers

## Q1: At-Least-Once vs Exactly-Once

The outbox worker performs two steps per entry:

1. **Publish** the event to the message broker.
2. **Mark processed** in the outbox table.

If the worker crashes after step 1 but before step 2, the entry remains unprocessed. On restart, the worker reads it again and publishes a second time. The downstream consumer receives a duplicate.

This is **at-least-once** delivery: every event is guaranteed to be delivered at least once, but duplicates are possible. It is not exactly-once because there is no atomic operation that spans both "publish to broker" and "mark processed in database."

**How consumers should handle this:**

- **Idempotent processing:** Consumers must tolerate receiving the same event more than once. Applying the same `OrderCompletedEvent` twice should not double-charge the customer or send duplicate notifications.
- **Deduplication by event ID:** Store processed event IDs (e.g., in a `processed_events` table). Before applying side effects, check whether the event ID has already been handled. Reject duplicates.
- **Idempotency keys for external calls:** When the consumer triggers external side effects (payment capture, email send), use the event ID or a derived idempotency key so the external system also rejects duplicates.

---

## Q2: Outbox Table Schema

```sql
CREATE TABLE outbox (
    id             STRING(36)    NOT NULL,
    aggregate_type STRING(100)   NOT NULL,
    aggregate_id   STRING(100)   NOT NULL,
    event_type     STRING(100)   NOT NULL,
    payload        BYTES(MAX)    NOT NULL,
    occurred_at    TIMESTAMP     NOT NULL,
    created_at     TIMESTAMP     NOT NULL,
    processed_at   TIMESTAMP,
    attempt_count  INT64         NOT NULL DEFAULT (0),
    last_error     STRING(1000),
) PRIMARY KEY (id);
```

**Indexes:**

| Index | Columns | Purpose |
|-------|---------|---------|
| `idx_outbox_pending` | `NULL_FILTERED on (created_at ASC) WHERE processed_at IS NULL`, storing key event columns | Worker scans for unprocessed entries in FIFO order. Spanner NULL_FILTERED index keeps the index small — processed entries are excluded, so the hot scan stays efficient as the table grows. |
| `idx_outbox_aggregate` | `(aggregate_type, aggregate_id, occurred_at ASC)` | Supports per-aggregate ordering queries and debugging. Ensures events for the same aggregate can be read in sequence. |

**Preventing duplicate processing:**

- The `processed_at` column (NULL = pending, non-NULL = done) controls the worker's read filter. Once set, the entry is excluded from future scans.
- Consumers maintain their own deduplication using the outbox `id` as the event's unique identity. A `processed_events` table or equivalent store on the consumer side records which IDs have been applied, rejecting replays.
- The `attempt_count` column tracks retries. Entries exceeding a threshold can be routed to a dead-letter mechanism for manual investigation rather than retrying forever.

---

## Q3: Out-of-Order Event Processing

**What goes wrong:**

The consumer processes `OrderCancelledEvent` (t=1) first and marks the order as cancelled. Then it processes `OrderCompletedEvent` (t=0) and marks the order as completed — overwriting the cancellation. The customer's order is now incorrectly in a completed state despite having been cancelled. Downstream effects (fulfillment, billing) act on stale information.

**How to ensure ordering:**

1. **Partition by aggregate ID.** Route all events for the same order to the same partition (Kafka partition key = order ID). This guarantees that a single consumer processes events for a given order in the order they were produced.

2. **Per-aggregate sequence numbers.** Add a `sequence` column to the outbox table, incremented per aggregate. The consumer tracks the last applied sequence per aggregate. If an event arrives with a sequence ≤ the last applied, it is rejected as stale or duplicate.

3. **Defensive state transitions.** The consumer should validate that the transition is legal given the current state. If the order is already cancelled, applying a completion event should be rejected — the consumer does not blindly overwrite state.

Partition-level ordering handles the common case. Sequence numbers and defensive transitions handle edge cases like reprocessing after consumer reassignment.

---

## Q4: 10 Million Unprocessed Events — Recovery Strategy

1. **Assess and triage.** Determine why the consumer was down and whether it can safely reprocess the full backlog. Check if any events are poison messages that would block processing.

2. **Batch processing with pagination.** Process entries in bounded batches (e.g., 500–1000 at a time), ordered by `created_at ASC`. This prevents unbounded memory use and allows progress tracking.

3. **Horizontal scaling.** If the outbox supports partitioned reads (e.g., by aggregate_id hash), run multiple worker instances processing different partitions concurrently.

4. **Monitor lag.** Track the oldest unprocessed entry's `created_at` as a lag metric. Alert if lag stops decreasing — this indicates a stuck or poison entry.

5. **Dead-letter poison messages.** Entries that fail repeatedly (check `attempt_count`) should be moved to a dead-letter table or flagged for manual review. Do not let one bad entry block the entire backlog.

6. **Rate-limit downstream pressure.** If publishing to a broker or calling downstream services, apply backpressure to avoid overwhelming them with a sudden burst of 10M events.

7. **Archive processed rows.** After recovery, clean up the outbox table. Move processed entries to an archive table or delete them to keep the pending scan index efficient. If the table is too large for efficient scans, consider partitioning by time or processed status.

8. **Post-mortem.** Add monitoring for consumer health and outbox backlog depth so a week-long outage is detected earlier next time.

---

## Q5: Bug with `time.Now()` in OutboxMuts

```go
CreatedAt: time.Now(),
```

**What's wrong:**

`time.Now()` in the repository layer generates a new wall-clock timestamp at outbox-entry construction time, which is disconnected from the business event's actual occurrence time. This causes several problems:

- **Clock skew between event and outbox.** The outbox `CreatedAt` will differ from the event's `CompletedAt` by however long the code took to reach this line. Under load or with retries, this drift can be significant.
- **Non-deterministic and untestable.** Tests cannot control or assert on the timestamp because it's generated internally by the repo. Each test run produces different values.
- **Ordering and audit inaccuracy.** If outbox entries are processed or debugged based on `CreatedAt`, the timestamp does not reflect when the business event actually occurred — it reflects when the persistence layer happened to construct the row.
- **Replay and retry inconsistency.** If the same event is retried through the repo layer, it gets a different `CreatedAt` each time, making deduplication and ordering reasoning harder.

**What it should be:**

Use the event's occurrence time — the timestamp that was set during the domain operation:

```go
CreatedAt: event.OccurredAt(),
```

This ensures the outbox entry's time is anchored to the business fact, controlled by the domain/usecase layer (via an injected clock), and deterministic in tests.
