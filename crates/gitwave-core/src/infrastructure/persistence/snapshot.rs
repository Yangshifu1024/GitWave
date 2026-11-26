//! A copy of the state database, for a presentation layer that must not open
//! the user's own database yet: the rewrite under development shows the real
//! workspaces from the copy and leaves the original alone.
//!
//! The copy is taken with SQLite's own `VACUUM INTO` through a read-only
//! connection. It is consistent while the original is being written, and it
//! holds what the original's write-ahead log holds. The original's
//! `state.db`, and a write-ahead log it already has, stay as they were. Two
//! files of SQLite's own can appear beside them: the `-shm` index, which
//! every reader reads or creates, and an empty `state.db-wal` when the
//! original was closed cleanly and had none. Neither holds anything, and the
//! application that owns the database removes them when it next closes.

use std::path::{Path, PathBuf};
use std::time::{Duration, SystemTime};

use rusqlite::{Connection, OpenFlags};
use serde::{Deserialize, Serialize};

use super::sqlite::{self, ensure_dir, lock_down_db_files, STATE_DB_FILE};
use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;

/// The file, beside the copy, that says which version of the original it holds.
const RECORD_FILE: &str = "state.snapshot.json";

/// The size and the modification time of one file.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
struct FileStamp {
    len: u64,
    /// Nanoseconds since the Unix epoch.
    modified_nanos: u64,
}

/// What the database in a state directory holds, as far as "did it change"
/// goes: the stamps of `state.db` and of its write-ahead log. Every change
/// lands in one of the two. The `-shm` index is left out on purpose: reading
/// the database touches it too, and so does an empty log.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Stamp {
    db: Option<FileStamp>,
    wal: Option<FileStamp>,
}

/// What came of bringing the copy up to date.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Refresh {
    /// The copy was taken, or taken again because the original had changed.
    Taken(SystemTime),
    /// The copy already holds the original as it is now.
    Current,
    /// There is no original. The directory keeps a database of its own.
    Fresh,
}

#[derive(Serialize, Deserialize)]
struct Record {
    stamp: Stamp,
    taken_at_unix_ms: u128,
}

impl Stamp {
    /// The stamp of the database in `dir`. Without a `state.db`, there is no
    /// database there.
    pub fn of(dir: &Path) -> Self {
        Self {
            db: file_stamp(&dir.join(STATE_DB_FILE)),
            // An empty log holds nothing, and taking a copy leaves one
            // beside the original: it is no log.
            wal: file_stamp(&dir.join(format!("{STATE_DB_FILE}-wal"))).filter(|wal| wal.len > 0),
        }
    }

    fn holds_a_database(&self) -> bool {
        self.db.is_some()
    }
}

fn file_stamp(path: &Path) -> Option<FileStamp> {
    let meta = std::fs::metadata(path).ok()?;
    // A file dated before the epoch counts as unchanged; nothing writes one.
    let modified_nanos = meta
        .modified()
        .ok()
        .and_then(|time| time.duration_since(SystemTime::UNIX_EPOCH).ok())
        .map_or(0, |since| since.as_nanos() as u64);
    Some(FileStamp {
        len: meta.len(),
        modified_nanos,
    })
}

/// Make `dir` hold the database in `source_dir` as it is now. A copy is
/// taken when there is none, when the original changed since the last one,
/// and when the record beside the copy is missing or unreadable.
///
/// The original is only read. See the module for the two files of SQLite's
/// own that reading it can leave beside it.
pub fn refresh(source_dir: &Path, dir: &Path) -> Result<Refresh> {
    let stamp = Stamp::of(source_dir);
    if !stamp.holds_a_database() {
        return Ok(Refresh::Fresh);
    }
    if read_record(dir).is_some_and(|record| record.stamp == stamp)
        && dir.join(STATE_DB_FILE).exists()
    {
        return Ok(Refresh::Current);
    }
    let taken_at = copy(source_dir, dir, &stamp)?;
    Ok(Refresh::Taken(taken_at))
}

/// When the database in `dir` was copied, for the one that was copied.
pub fn taken_at(dir: &Path) -> Option<SystemTime> {
    read_record(dir).map(|record| {
        SystemTime::UNIX_EPOCH + Duration::from_millis(record.taken_at_unix_ms as u64)
    })
}

fn read_record(dir: &Path) -> Option<Record> {
    let raw = std::fs::read_to_string(dir.join(RECORD_FILE)).ok()?;
    serde_json::from_str(&raw).ok()
}

fn copy(source_dir: &Path, dir: &Path, stamp: &Stamp) -> Result<SystemTime> {
    ensure_dir(dir)?;
    let original = source_dir.join(STATE_DB_FILE);
    let staging = dir.join(format!("{STATE_DB_FILE}.{}.copying", std::process::id()));
    let _ = std::fs::remove_file(&staging);
    vacuum_into(&original, &staging)?;
    // From here on the staging file holds the whole state, so it is locked
    // down at once and never left behind.
    lock_down_db_files(&staging);
    let taken_at = install(&staging, dir, stamp);
    if taken_at.is_err() {
        let _ = std::fs::remove_file(&staging);
    }
    taken_at
}

/// Puts the finished copy in place of the old one, and records what it
/// holds. Returns when it was taken.
fn install(staging: &Path, dir: &Path, stamp: &Stamp) -> Result<SystemTime> {
    // The log beside the old copy belongs to the old copy. SQLite would
    // replay it onto the new database, so it has to go before the new one
    // takes its place. The index is rebuilt when it is missing.
    for sidecar in ["-wal", "-journal"] {
        let path = dir.join(format!("{STATE_DB_FILE}{sidecar}"));
        if path.exists() {
            std::fs::remove_file(&path)
                .map_err(|e| failed(format!("remove {}: {e}", path.display())))?;
        }
    }
    let _ = std::fs::remove_file(dir.join(format!("{STATE_DB_FILE}-shm")));
    let copy = dir.join(STATE_DB_FILE);
    std::fs::rename(staging, &copy).map_err(|e| failed(format!("move {}: {e}", copy.display())))?;
    lock_down_db_files(&copy);
    // To the millisecond, because that is what the record keeps: the time
    // handed back and the time read back are then the same.
    let taken_at_unix_ms = SystemTime::now()
        .duration_since(SystemTime::UNIX_EPOCH)
        .map_or(0, |since| since.as_millis());
    let taken_at = SystemTime::UNIX_EPOCH + Duration::from_millis(taken_at_unix_ms as u64);
    let record = Record {
        stamp: stamp.clone(),
        taken_at_unix_ms,
    };
    let raw =
        serde_json::to_string(&record).map_err(|e| failed(format!("serialize the record: {e}")))?;
    std::fs::write(dir.join(RECORD_FILE), raw)
        .map_err(|e| failed(format!("write the record: {e}")))?;
    Ok(taken_at)
}

/// Write a consistent copy of the database at `original` to `target`, and
/// leave nothing behind when that fails.
fn vacuum_into(original: &Path, target: &Path) -> Result<()> {
    // Read-only: the original is never written to, not even by the
    // checkpoint SQLite runs when the last connection to it closes.
    let copied: std::result::Result<(), String> = (|| {
        let conn = Connection::open_with_flags(original, OpenFlags::SQLITE_OPEN_READ_ONLY)
            .map_err(|e| format!("open the original: {e}"))?;
        conn.busy_timeout(Duration::from_secs(5))
            .map_err(|e| format!("wait for the original: {e}"))?;
        // The statement takes the path as a literal, so quotes in it are doubled.
        let path = target.to_string_lossy().replace('\'', "''");
        conn.execute(&format!("VACUUM INTO '{path}'"), [])
            .map_err(|e| format!("copy the original: {e}"))?;
        Ok(())
    })();
    if let Err(error) = copied {
        let _ = std::fs::remove_file(target);
        return Err(failed(error));
    }
    Ok(())
}

fn failed(error: impl std::fmt::Display) -> AppError {
    AppError::unknown_with(
        codes::infra::STATE_SNAPSHOT_FAILED,
        format!("the state database could not be copied, {error}"),
        &[("error", error.to_string())],
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::application::use_cases::{create_workspace, list_workspaces};
    use crate::bootstrap::open_app_context_in;

    fn names(ctx: &crate::application::use_cases::AppContext) -> Vec<String> {
        list_workspaces(ctx)
            .expect("list the workspaces")
            .into_iter()
            .map(|workspace| workspace.name)
            .collect()
    }

    /// A short digest of a file, to show it was not written to.
    fn digest(path: &Path) -> Option<Vec<u8>> {
        let bytes = std::fs::read(path).ok()?;
        let mut sum = 0u64;
        for (ix, byte) in bytes.iter().enumerate() {
            sum = sum.rotate_left(7) ^ (u64::from(*byte) + ix as u64);
        }
        Some(sum.to_be_bytes().to_vec())
    }

    /// The original is open elsewhere, as the released application leaves it,
    /// with its latest changes still in the write-ahead log.
    #[test]
    fn the_copy_holds_what_the_log_holds_and_the_original_stays_as_it_was() {
        let tmp = tempfile::tempdir().expect("a directory of the test's own");
        let tmp = tmp.path().to_path_buf();
        let (original, copy) = (tmp.join("real"), tmp.join("dev"));
        let released = open_app_context_in(&original);
        create_workspace(&released, "One".to_string()).expect("create a workspace");

        let before = (
            digest(&original.join(STATE_DB_FILE)),
            digest(&original.join(format!("{STATE_DB_FILE}-wal"))),
        );
        let Refresh::Taken(at) = refresh(&original, &copy).expect("take a copy") else {
            panic!("there was no copy to begin with");
        };
        assert!(at <= SystemTime::now(), "the copy is dated");

        assert_eq!(
            (
                digest(&original.join(STATE_DB_FILE)),
                digest(&original.join(format!("{STATE_DB_FILE}-wal")))
            ),
            before,
            "the original's database and log are not written to"
        );
        assert_eq!(
            names(&open_app_context_in(&copy)),
            ["One"],
            "the copy holds the log's changes"
        );
        assert_eq!(taken_at(&copy), Some(at), "the copy says when it was taken");
        assert!(
            matches!(refresh(&original, &copy), Ok(Refresh::Current)),
            "nothing changed"
        );

        // The released application goes on working while the copy exists.
        create_workspace(&released, "Two".to_string()).expect("create another workspace");
        assert!(
            matches!(refresh(&original, &copy), Ok(Refresh::Taken(_))),
            "the original changed"
        );
        let names = names(&open_app_context_in(&copy));
        assert!(
            names.contains(&"Two".to_string()),
            "the new copy holds the change: {names:?}"
        );
    }

    #[test]
    fn without_an_original_the_directory_keeps_a_database_of_its_own() {
        let tmp = tempfile::tempdir().expect("a directory of the test's own");
        let tmp = tmp.path().to_path_buf();
        let (original, copy) = (tmp.join("real"), tmp.join("dev"));

        assert!(matches!(refresh(&original, &copy), Ok(Refresh::Fresh)));
        let own = open_app_context_in(&copy);
        create_workspace(&own, "Mine".to_string()).expect("create a workspace");

        assert!(
            matches!(refresh(&original, &copy), Ok(Refresh::Fresh)),
            "still no original"
        );
        assert_eq!(
            names(&open_app_context_in(&copy)),
            ["Mine"],
            "what was stored there is kept"
        );
        assert_eq!(taken_at(&copy), None, "nothing was copied");
    }

    #[test]
    fn an_unreadable_original_is_reported_and_the_copy_stays() {
        let tmp = tempfile::tempdir().expect("a directory of the test's own");
        let tmp = tmp.path().to_path_buf();
        let (original, copy) = (tmp.join("real"), tmp.join("dev"));
        let released = open_app_context_in(&original);
        create_workspace(&released, "One".to_string()).expect("create a workspace");
        refresh(&original, &copy).expect("take a copy");
        let kept = digest(&copy.join(STATE_DB_FILE));
        drop(released);

        std::fs::write(original.join(STATE_DB_FILE), b"not a database at all")
            .expect("break the original");
        let error = refresh(&original, &copy).expect_err("the original cannot be copied");

        assert_eq!(error.code(), codes::infra::STATE_SNAPSHOT_FAILED);
        assert_eq!(
            digest(&copy.join(STATE_DB_FILE)),
            kept,
            "the copy that worked is kept"
        );
        assert_eq!(names(&open_app_context_in(&copy)), ["One"]);
        let leftovers: Vec<_> = std::fs::read_dir(&copy)
            .expect("list the copy's directory")
            .flatten()
            .map(|entry| entry.file_name().to_string_lossy().into_owned())
            .filter(|name| name.ends_with(".copying"))
            .collect();
        assert!(
            leftovers.is_empty(),
            "no half-made copy is left behind: {leftovers:?}"
        );
    }

    #[test]
    fn a_copy_that_is_gone_is_taken_again() {
        let tmp = tempfile::tempdir().expect("a directory of the test's own");
        let tmp = tmp.path().to_path_buf();
        let (original, copy) = (tmp.join("real"), tmp.join("dev"));
        let released = open_app_context_in(&original);
        create_workspace(&released, "One".to_string()).expect("create a workspace");
        refresh(&original, &copy).expect("take a copy");
        std::fs::remove_file(copy.join(STATE_DB_FILE)).expect("lose the copy");

        assert!(
            matches!(refresh(&original, &copy), Ok(Refresh::Taken(_))),
            "the record alone is not a copy"
        );
        assert_eq!(names(&open_app_context_in(&copy)), ["One"]);
    }

    #[test]
    fn a_record_that_cannot_be_read_is_taken_as_absent() {
        let tmp = tempfile::tempdir().expect("a directory of the test's own");
        let tmp = tmp.path().to_path_buf();
        let (original, copy) = (tmp.join("real"), tmp.join("dev"));
        let released = open_app_context_in(&original);
        create_workspace(&released, "One".to_string()).expect("create a workspace");
        refresh(&original, &copy).expect("take a copy");
        std::fs::write(copy.join(RECORD_FILE), b"{ not json").expect("break the record");

        assert!(
            matches!(refresh(&original, &copy), Ok(Refresh::Taken(_))),
            "the copy is made again"
        );
        assert!(taken_at(&copy).is_some(), "and recorded again");
    }

    /// The usual state of the original: its application closed, which removes
    /// its log. Reading it leaves an empty one beside it, which is no change,
    /// or the copy would be taken again after every run of the released
    /// application.
    #[test]
    fn an_empty_log_left_beside_the_original_by_the_copy_is_no_change() {
        let tmp = tempfile::tempdir().expect("a directory of the test's own");
        let tmp = tmp.path().to_path_buf();
        let (original, copy) = (tmp.join("real"), tmp.join("dev"));
        let released = open_app_context_in(&original);
        create_workspace(&released, "One".to_string()).expect("create a workspace");
        drop(released);
        let log = original.join(format!("{STATE_DB_FILE}-wal"));
        let clean_close = !log.exists();
        let before = digest(&original.join(STATE_DB_FILE));

        let Refresh::Taken(_) = refresh(&original, &copy).expect("take a copy") else {
            panic!("there was no copy to begin with");
        };

        // What reading leaves beside the original is SQLite's to tell: this
        // records it, so that a change in SQLite shows up here.
        let left_behind = std::fs::metadata(&log).map(|meta| meta.len()).ok();
        if clean_close {
            assert_eq!(left_behind, Some(0), "an empty log, and no more");
        }
        assert_eq!(
            digest(&original.join(STATE_DB_FILE)),
            before,
            "the original's database is not written to"
        );
        assert_eq!(
            names(&open_app_context_in(&copy)),
            ["One"],
            "the copy holds what the original holds"
        );

        assert!(
            matches!(refresh(&original, &copy), Ok(Refresh::Current)),
            "what the copy left beside the original is no change: the copy is not taken again"
        );
    }

    /// The application that owns the original opens it, works, and closes
    /// after the copy left files beside it.
    #[test]
    fn the_original_can_be_worked_with_after_a_copy_was_taken() {
        let tmp = tempfile::tempdir().expect("a directory of the test's own");
        let tmp = tmp.path().to_path_buf();
        let (original, copy) = (tmp.join("real"), tmp.join("dev"));
        let released = open_app_context_in(&original);
        create_workspace(&released, "One".to_string()).expect("create a workspace");
        drop(released);

        let Refresh::Taken(_) = refresh(&original, &copy).expect("take a copy") else {
            panic!("there was no copy to begin with");
        };
        let released = open_app_context_in(&original);
        create_workspace(&released, "Two".to_string()).expect("create another workspace");
        let mut worked_in = names(&released);
        worked_in.sort();
        assert_eq!(worked_in, ["One", "Two"]);
        drop(released);

        assert!(
            matches!(refresh(&original, &copy), Ok(Refresh::Taken(_))),
            "the original changed while it was worked with"
        );
        assert!(names(&open_app_context_in(&copy)).contains(&"Two".to_string()));
    }

    /// The copy is made but cannot be put in place: the whole state it holds
    /// is not left lying in the directory.
    #[test]
    fn a_copy_that_cannot_be_put_in_place_is_not_left_behind() {
        let tmp = tempfile::tempdir().expect("a directory of the test's own");
        let tmp = tmp.path().to_path_buf();
        let (original, copy) = (tmp.join("real"), tmp.join("dev"));
        let released = open_app_context_in(&original);
        create_workspace(&released, "One".to_string()).expect("create a workspace");
        std::fs::create_dir_all(copy.join(STATE_DB_FILE).join("in the way"))
            .expect("a directory where the copy would go");

        let error = refresh(&original, &copy)
            .expect_err("a file cannot replace a directory that is not empty");

        assert_eq!(error.code(), codes::infra::STATE_SNAPSHOT_FAILED);
        let leftovers: Vec<_> = std::fs::read_dir(&copy)
            .expect("list the copy's directory")
            .flatten()
            .map(|entry| entry.file_name().to_string_lossy().into_owned())
            .filter(|name| name.ends_with(".copying"))
            .collect();
        assert!(
            leftovers.is_empty(),
            "the copy that was made is removed: {leftovers:?}"
        );
    }

    #[test]
    fn quotes_in_the_paths_are_nothing_special() {
        let tmp = tempfile::tempdir().expect("a directory of the test's own");
        let tmp = tmp.path().to_path_buf();
        let (original, copy) = (tmp.join("it's real"), tmp.join("it's ' copied"));
        let released = open_app_context_in(&original);
        create_workspace(&released, "One".to_string()).expect("create a workspace");

        refresh(&original, &copy).expect("take a copy");

        assert_eq!(names(&open_app_context_in(&copy)), ["One"]);
    }
}
