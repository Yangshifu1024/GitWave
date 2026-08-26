-- Sprint 2: repos status + missing tracking.
-- See docs/pm/features/F002-repo-ingestion.md.

ALTER TABLE repos ADD COLUMN status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE repos ADD COLUMN missing_at INTEGER;