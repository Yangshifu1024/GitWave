//! Stash domain types.

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct StashEntry {
    pub index: u32,
    pub message: String,
    pub oid: String,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn stash_entry_roundtrip() {
        let e = StashEntry {
            index: 0,
            message: "WIP".into(),
            oid: "abc".into(),
        };
        let json = serde_json::to_string(&e).unwrap();
        let back: StashEntry = serde_json::from_str(&json).unwrap();
        assert_eq!(back.message, "WIP");
    }
}
