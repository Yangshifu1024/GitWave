//! GitWave application core: the domain, use-case and infrastructure layers
//! every presentation layer shares, plus the startup and host integration
//! that need no UI framework. Nothing here may depend on a UI framework.
//!
//! Architecture follows `docs/tech/architecture/00-overview.md`:
//! presentation → application → domain ← infrastructure.

// Carried over from the Tauri crate root, where the command layer and these
// layers were compiled as one crate. Lifted per module, not wholesale.
#![allow(dead_code)]
#![allow(unused_imports)]

pub mod application;
pub mod bootstrap;
pub mod domain;
pub mod host;
pub mod infrastructure;

#[cfg(test)]
mod tests {
    /// The core is shared by every presentation layer, so it must stay free
    /// of UI frameworks; a dependency on one would couple all of them to it.
    #[test]
    fn core_does_not_depend_on_a_ui_framework() {
        let manifest = include_str!("../Cargo.toml");
        for framework in ["tauri", "gpui"] {
            assert!(
                !manifest.contains(framework),
                "gitwave-core must not depend on {framework}"
            );
        }
    }

    /// Without its native feature, keyring silently substitutes an in-memory
    /// mock: saving AI keys and Git credentials then appears to work until the
    /// app restarts. Every supported platform needs its store declared.
    #[test]
    fn every_platform_declares_its_native_credential_store() {
        let manifest = include_str!("../Cargo.toml");
        for backend in ["apple-native", "windows-native", "sync-secret-service"] {
            assert!(
                manifest.contains(&format!("features = [\"{backend}\"]")),
                "no platform declares keyring's {backend} backend"
            );
        }
    }

    /// Creating an entry stores nothing, so this reveals the backend keyring
    /// would use without touching the user's secrets.
    #[test]
    fn native_credential_store_is_compiled_in() {
        let Ok(entry) = keyring::Entry::new("gitwave-core-probe", "probe") else {
            // The Secret Service is unreachable without a session bus. The
            // mock never fails here, so a fallback is still caught.
            return;
        };
        let backend = format!("{entry:?}");
        assert!(
            !backend.contains("MockCredential"),
            "keyring fell back to its in-memory mock: {backend}"
        );
    }
}
