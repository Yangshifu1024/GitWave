-- F005: explicit tab ordering for repos.
-- See docs/pm/features/F005-repo-tab-drag-reorder.md.

ALTER TABLE repos ADD COLUMN position INTEGER NOT NULL DEFAULT 0;

-- Backfill: preserve the pre-migration listing order (added_at, id as
-- tiebreaker) so existing users see no change before their first reorder.
UPDATE repos SET position = (
    SELECT COUNT(*) FROM repos AS r2
    WHERE r2.workspace_id = repos.workspace_id
      AND (r2.added_at < repos.added_at
           OR (r2.added_at = repos.added_at AND r2.id <= repos.id))
) - 1;
