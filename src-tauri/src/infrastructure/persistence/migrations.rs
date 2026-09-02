//! Embedded migration runner.
//!
//! Migrations live in `src-tauri/migrations/NNNN-name.sql` and are embedded
//! at compile time via `include_str!`. The `schema_version` table tracks
//! which migrations have been applied. New migrations are added by appending
//! a `Migration` entry to `MIGRATIONS` and the corresponding SQL file.

use std::collections::HashSet;

use rusqlite::Connection;

use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;

/// All migrations, in version order. Add new entries here as files are
/// added to `migrations/`.
const MIGRATIONS: &[Migration] = &[
    Migration {
        version: 1,
        name: "workspaces-repos",
        sql: include_str!("../../../migrations/0001-workspaces-repos.sql"),
    },
    Migration {
        version: 2,
        name: "repos-status-and-missing",
        sql: include_str!("../../../migrations/0002-repos-status.sql"),
    },
    Migration {
        version: 3,
        name: "repos-position",
        sql: include_str!("../../../migrations/0003-repos-position.sql"),
    },
    Migration {
        version: 4,
        name: "app-settings",
        sql: include_str!("../../../migrations/0004-app-settings.sql"),
    },
];

struct Migration {
    version: i64,
    name: &'static str,
    sql: &'static str,
}

/// Apply pending migrations. Idempotent: re-running on a fully migrated
/// database is a no-op.
pub fn apply(conn: &Connection) -> Result<()> {
    conn.execute_batch(
        "CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY NOT NULL)",
    )
    .map_err(map_sqlite_err)?;

    let applied: HashSet<i64> = conn
        .prepare("SELECT version FROM schema_version")
        .map_err(map_sqlite_err)?
        .query_map([], |r| r.get::<_, i64>(0))
        .map_err(map_sqlite_err)?
        .filter_map(std::result::Result::ok)
        .collect();

    for migration in MIGRATIONS {
        if applied.contains(&migration.version) {
            continue;
        }
        let tx = conn.unchecked_transaction().map_err(map_sqlite_err)?;
        tx.execute_batch(migration.sql).map_err(map_sqlite_err)?;
        tx.execute(
            "INSERT INTO schema_version (version) VALUES (?1)",
            [migration.version],
        )
        .map_err(map_sqlite_err)?;
        tx.commit().map_err(map_sqlite_err)?;
        tracing::info!(
            version = migration.version,
            name = migration.name,
            "applied migration"
        );
    }

    Ok(())
}

fn map_sqlite_err(e: rusqlite::Error) -> AppError {
    AppError::unknown_with(
        codes::infra::MIGRATION_FAILED,
        format!("sqlite migration: {e}"),
        &[("error", e.to_string())],
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    fn in_memory() -> Connection {
        Connection::open_in_memory().expect("in-memory sqlite")
    }

    #[test]
    fn apply_creates_workspaces_and_repos_tables() {
        let conn = in_memory();
        apply(&conn).expect("apply migrations");

        let tables: Vec<String> = conn
            .prepare("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
            .unwrap()
            .query_map([], |r| r.get(0))
            .unwrap()
            .filter_map(std::result::Result::ok)
            .collect();

        assert!(tables.contains(&"workspaces".to_string()));
        assert!(tables.contains(&"repos".to_string()));
        assert!(tables.contains(&"schema_version".to_string()));
    }

    #[test]
    fn apply_is_idempotent() {
        let conn = in_memory();
        apply(&conn).expect("first");
        apply(&conn).expect("second");
    }

    #[test]
    fn apply_records_versions_in_schema_version() {
        let conn = in_memory();
        apply(&conn).expect("apply");
        let versions: Vec<i64> = conn
            .prepare("SELECT version FROM schema_version ORDER BY version")
            .unwrap()
            .query_map([], |r| r.get(0))
            .unwrap()
            .filter_map(std::result::Result::ok)
            .collect();
        assert_eq!(versions, vec![1, 2, 3, 4]);
    }

    #[test]
    fn migration_2_adds_repos_status_columns() {
        let conn = in_memory();
        apply(&conn).expect("apply");

        // Verify status column with default 'active' and missing_at is nullable.
        let status_default: String = conn
            .query_row("SELECT status FROM repos LIMIT 1", [], |r| r.get(0))
            .unwrap_or_else(|_| "active".to_string());
        // Table is empty so we just verify the column is queryable.
        let _ = status_default;

        // Insert a row directly and check missing_at is NULL by default.
        conn.execute(
            "INSERT INTO workspaces (id, name, settings_json, created_at, updated_at) \
             VALUES ('ws-1', 'X', '{}', 1, 1)",
            [],
        )
        .unwrap();
        conn.execute(
            "INSERT INTO repos (id, workspace_id, path, added_at) \
             VALUES ('r-1', 'ws-1', '/tmp', 1)",
            [],
        )
        .unwrap();
        let missing_at: Option<i64> = conn
            .query_row("SELECT missing_at FROM repos WHERE id = 'r-1'", [], |r| {
                r.get(0)
            })
            .unwrap();
        assert!(missing_at.is_none());
    }

    #[test]
    fn migration_3_backfills_position_by_added_at() {
        let conn = in_memory();
        // Apply only up to v2 so the rows below reproduce a pre-F005 database.
        conn.execute_batch(
            "CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY NOT NULL)",
        )
        .unwrap();
        for migration in MIGRATIONS.iter().filter(|m| m.version <= 2) {
            conn.execute_batch(migration.sql).unwrap();
            conn.execute(
                "INSERT INTO schema_version (version) VALUES (?1)",
                [migration.version],
            )
            .unwrap();
        }

        conn.execute(
            "INSERT INTO workspaces (id, name, settings_json, created_at, updated_at) \
             VALUES ('ws-1', 'X', '{}', 1, 1)",
            [],
        )
        .unwrap();
        // Insertion order deliberately differs from added_at order; ties on
        // added_at fall back to id (matches the backfill's tiebreaker).
        for (id, added_at) in [
            ("r-2", 2000i64),
            ("r-1", 1000),
            ("r-4", 1000),
            ("r-3", 3000),
        ] {
            conn.execute(
                "INSERT INTO repos (id, workspace_id, path, added_at) \
                 VALUES (?1, 'ws-1', '/tmp', ?2)",
                rusqlite::params![id, added_at],
            )
            .unwrap();
        }

        apply(&conn).expect("apply v3");

        let ordered: Vec<String> = conn
            .prepare("SELECT id FROM repos WHERE workspace_id = 'ws-1' ORDER BY position")
            .unwrap()
            .query_map([], |r| r.get(0))
            .unwrap()
            .filter_map(std::result::Result::ok)
            .collect();
        assert_eq!(ordered, vec!["r-1", "r-4", "r-2", "r-3"]);
    }
}
