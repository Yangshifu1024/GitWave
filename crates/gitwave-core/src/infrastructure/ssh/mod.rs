//! SSH key management — wrappers around the system `ssh` / `ssh-add`
//! binaries. See `docs/tech/decisions/0003` (credential strategy: ssh-agent
//! handles SSH, no local plaintext storage).

pub mod keys;
