//! Infrastructure layer — adapters for external capabilities.
//!
//! Concrete use cases consume these via `application::AppContext`.

pub mod git;
pub mod observability;
pub mod persistence;
