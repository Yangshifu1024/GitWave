-- GitWave Sprint 1 schema: workspaces + repos.
-- See docs/pm/features/F001-workspace-crud.md and
-- docs/tech/decisions/0002 (Workspace as abstract, no FS entity).

CREATE TABLE workspaces (
    id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL,
    settings_json TEXT NOT NULL DEFAULT '{}',
    last_active_repo_id TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX idx_workspaces_updated_at ON workspaces(updated_at DESC);

CREATE TABLE repos (
    id TEXT PRIMARY KEY NOT NULL,
    workspace_id TEXT NOT NULL,
    path TEXT NOT NULL,
    nickname TEXT,
    settings_override_json TEXT,
    added_at INTEGER NOT NULL,
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

CREATE INDEX idx_repos_workspace_id ON repos(workspace_id);