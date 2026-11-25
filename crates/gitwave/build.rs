//! Puts a digest of the message files into the crate's fingerprint.
//!
//! `rust_i18n::i18n!` reads `locales/` while the crate is compiled. Cargo only
//! tracks the files that rustc or a build script names, and the macro reads
//! these on its own, so without this an edited message would stay invisible
//! until something else caused a rebuild.

use std::hash::{Hash, Hasher};
use std::path::Path;

fn main() {
    let dir = Path::new(env!("CARGO_MANIFEST_DIR")).join("locales");
    // The first line makes cargo run this script again when a file under
    // `locales/` changes. The second is what the crate's fingerprint depends
    // on: an environment variable for rustc, so it changes with the files.
    println!("cargo:rerun-if-changed={}", dir.display());
    println!(
        "cargo:rustc-env=GITWAVE_LOCALES_DIGEST={:016x}",
        digest(&dir)
    );
}

/// A hash over every file name and content, in file name order, so that it
/// changes only when a message file does.
fn digest(dir: &Path) -> u64 {
    let mut entries: Vec<_> = std::fs::read_dir(dir)
        .expect("read the locales directory")
        .collect();
    entries.sort_by_key(|entry| {
        entry
            .as_ref()
            .map(|entry| entry.file_name())
            .unwrap_or_default()
    });
    let mut hasher = std::collections::hash_map::DefaultHasher::new();
    for entry in entries {
        let path = entry.expect("a locales entry").path();
        path.file_name().expect("a file name").hash(&mut hasher);
        std::fs::read(&path)
            .expect("read a message file")
            .hash(&mut hasher);
    }
    hasher.finish()
}
