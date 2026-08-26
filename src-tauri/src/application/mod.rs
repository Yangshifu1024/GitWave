//! Application layer — orchestrates domain operations, enforces use cases.
//!
//! Concrete use cases (e.g., `CreateWorkspace`, `SwitchActiveRepo`) land
//! here in subsequent sprints.

/// Placeholder for the application-layer context that ties together
/// infrastructure adapters (DB, git, AI) and exposes use cases to the
/// presentation layer.
pub struct AppContext;

impl AppContext {
    #[must_use]
    pub fn new() -> Self {
        Self
    }
}

impl Default for AppContext {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn app_context_constructs() {
        let _ = AppContext::new();
        let _ = AppContext;
    }
}
