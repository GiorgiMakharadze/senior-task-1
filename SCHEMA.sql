-- Outbox table for reliable event delivery via the transactional outbox pattern.
-- Written in a Spanner-compatible dialect. Adapt types for PostgreSQL/MySQL as needed.
-- Таблица outbox для надёжной доставки событий через transactional outbox паттерн.
-- Написано с использованием диалекта, совместимого со Spanner. При необходимости адаптируйте типы для PostgreSQL/MySQL.

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

-- Worker reads pending entries in FIFO order.
-- NULL processed_at = not yet processed.
-- Spanner NULL-filtered index keeps the index small as entries are processed.
-- Воркер читает необработанные записи в порядке FIFO.
-- processed_at = NULL означает, что событие ещё не обработано.
-- NULL-filtered индекс в Spanner уменьшает размер индекса по мере обработки записей.
CREATE NULL_FILTERED INDEX idx_outbox_pending
    ON outbox (created_at ASC)
    STORING (aggregate_type, aggregate_id, event_type, payload, occurred_at, attempt_count)
    WHERE processed_at IS NULL;

-- Per-aggregate ordering and debugging.
-- Supports reading all events for a given aggregate in sequence.
-- Индекс для упорядочивания событий по агрегату и удобства отладки.
-- Позволяет читать все события конкретного агрегата в правильной последовательности.
CREATE INDEX idx_outbox_aggregate
    ON outbox (aggregate_type, aggregate_id, occurred_at ASC);
