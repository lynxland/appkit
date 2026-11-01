-- +goose Up
CREATE TABLE IF NOT EXISTS outbox_events (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    aggregate    TEXT,
    payload      BYTEA NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    attempts     INTEGER NOT NULL DEFAULT 0,
    dead         BOOLEAN NOT NULL DEFAULT FALSE,
    claimed_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_outbox_pending ON outbox_events (created_at)
    WHERE published_at IS NULL AND dead = FALSE AND claimed_at IS NULL;
