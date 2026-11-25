-- F013: app-level global settings (first use: proxy configuration).
-- One row per setting key; the value is a JSON blob so new fields never
-- need another migration. See docs/pm/features/F013-system-proxy.md.

CREATE TABLE app_settings (
    key TEXT PRIMARY KEY NOT NULL,
    value_json TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
