//! What the shell is showing: the workspaces, one of them, its repositories,
//! and which of them is active.

use gitwave_core::domain::workspace::{RepoRef, RepoStatus, WorkspaceSummary};

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct Session {
    workspaces: Vec<WorkspaceSummary>,
    selected: Option<Selected>,
}

/// The workspace on screen and what is known about its repositories.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Selected {
    pub id: String,
    /// The repositories, in the order the tabs show them. `None` until they
    /// were read.
    pub repos: Option<Vec<RepoRef>>,
    /// The active one, as an index into the repositories.
    pub active: Option<usize>,
}

impl Session {
    /// A session with these workspaces. The one named by `remembered` is
    /// shown when it is still there, else the first one.
    pub fn new(workspaces: Vec<WorkspaceSummary>, remembered: Option<&str>) -> Self {
        let shown = workspaces
            .iter()
            .find(|workspace| Some(workspace.id.as_str()) == remembered)
            .or_else(|| workspaces.first())
            .map(|workspace| workspace.id.clone());
        Self {
            workspaces,
            selected: shown.map(|id| Selected {
                id,
                repos: None,
                active: None,
            }),
        }
    }

    pub fn workspaces(&self) -> &[WorkspaceSummary] {
        &self.workspaces
    }

    pub fn selected(&self) -> Option<&Selected> {
        self.selected.as_ref()
    }

    /// The id of the workspace on screen.
    pub fn selected_id(&self) -> Option<&str> {
        self.selected.as_ref().map(|selected| selected.id.as_str())
    }

    /// Shows another workspace. Its repositories are not known yet. `false`
    /// when there is no workspace with that id.
    pub fn show(&mut self, workspace_id: &str) -> bool {
        if !self
            .workspaces
            .iter()
            .any(|workspace| workspace.id == workspace_id)
        {
            return false;
        }
        self.selected = Some(Selected {
            id: workspace_id.to_string(),
            repos: None,
            active: None,
        });
        true
    }

    /// Takes the repositories as they were read. `false` when another
    /// workspace is shown by now, so that the caller drops them.
    pub fn set_repos(&mut self, workspace_id: &str, repos: Vec<RepoRef>) -> bool {
        let Some(selected) = self.selected.as_mut() else {
            return false;
        };
        if selected.id != workspace_id {
            return false;
        }
        // Read again: the active repository stays active when it is still there.
        selected.active = selected
            .active
            .and_then(|index| selected.repos.as_ref()?.get(index))
            .and_then(|active| repos.iter().position(|repo| repo.id == active.id));
        selected.repos = Some(repos);
        true
    }

    /// Makes `target` the active repository, or the first one that is where
    /// it was left when `target` cannot be shown. Returns what became active.
    pub fn restore_active(&mut self, target: Option<&str>) -> Option<RepoRef> {
        let selected = self.selected.as_mut()?;
        let repos = selected.repos.as_deref()?;
        selected.active = pick_active(repos, target);
        selected.active.and_then(|index| repos.get(index).cloned())
    }

    /// Makes the repository active. One whose path is missing is not
    /// activated: nothing would load for it.
    pub fn activate(&mut self, repo_id: &str) -> bool {
        let Some(selected) = self.selected.as_mut() else {
            return false;
        };
        let Some(repos) = selected.repos.as_deref() else {
            return false;
        };
        let Some(index) = repos
            .iter()
            .position(|repo| repo.id == repo_id && repo.status != RepoStatus::Missing)
        else {
            return false;
        };
        selected.active = Some(index);
        true
    }

    /// Notes that the active repository is the one to show for its workspace
    /// the next time it is shown, or the application starts. `false` when
    /// that is already what is noted, so that nothing is stored for it.
    pub fn keep_active(&mut self) -> bool {
        let Some(selected) = &self.selected else {
            return false;
        };
        let Some(active) = selected
            .active
            .and_then(|index| selected.repos.as_deref()?.get(index))
        else {
            return false;
        };
        let Some(workspace) = self
            .workspaces
            .iter_mut()
            .find(|workspace| workspace.id == selected.id)
        else {
            return false;
        };
        if workspace.last_active_repo_id.as_deref() == Some(active.id.as_str()) {
            return false;
        }
        workspace.last_active_repo_id = Some(active.id.clone());
        true
    }

    pub fn active_repo(&self) -> Option<&RepoRef> {
        let selected = self.selected.as_ref()?;
        selected
            .active
            .and_then(|index| selected.repos.as_deref()?.get(index))
    }
}

/// The repository to show: `target` when it is there and where it was left,
/// else the first one that is.
fn pick_active(repos: &[RepoRef], target: Option<&str>) -> Option<usize> {
    let target = target.and_then(|target| {
        repos
            .iter()
            .position(|repo| repo.id == target && repo.status != RepoStatus::Missing)
    });
    target.or_else(|| {
        repos
            .iter()
            .position(|repo| repo.status != RepoStatus::Missing)
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn workspace(id: &str) -> WorkspaceSummary {
        WorkspaceSummary {
            id: id.to_string(),
            name: id.to_string(),
            last_active_repo_id: None,
            updated_at: 0,
        }
    }

    fn repo(id: &str, status: RepoStatus) -> RepoRef {
        RepoRef {
            id: id.to_string(),
            workspace_id: "ws".to_string(),
            path: format!("/{id}"),
            nickname: None,
            settings_override: None,
            status,
            missing_since: None,
            added_at: 0,
        }
    }

    /// A session on a workspace whose repositories were read.
    fn with_repos(repos: Vec<RepoRef>) -> Session {
        let mut session = Session::new(vec![workspace("ws")], None);
        assert!(session.set_repos("ws", repos), "the workspace is shown");
        session
    }

    #[test]
    fn the_remembered_workspace_is_shown_when_it_is_still_there() {
        let workspaces = vec![workspace("a"), workspace("b")];
        assert_eq!(
            Session::new(workspaces.clone(), Some("b")).selected_id(),
            Some("b")
        );
        assert_eq!(
            Session::new(workspaces, Some("gone")).selected_id(),
            Some("a"),
            "else the first one"
        );
        assert_eq!(
            Session::new(vec![], Some("a")).selected_id(),
            None,
            "nothing to show"
        );
    }

    #[test]
    fn showing_a_workspace_forgets_what_was_known_about_the_previous_one() {
        let mut session = with_repos(vec![repo("r", RepoStatus::Active)]);
        assert!(session.restore_active(Some("r")).is_some());

        assert!(!session.show("gone"), "an unknown workspace is not shown");
        assert!(session.show("ws"));
        assert_eq!(
            session.selected().map(|selected| selected.repos.clone()),
            Some(None)
        );
        assert_eq!(session.active_repo(), None);
    }

    #[test]
    fn repositories_read_for_another_workspace_are_refused() {
        let mut session = Session::new(vec![workspace("a"), workspace("b")], Some("a"));

        assert!(!session.set_repos("b", vec![repo("r", RepoStatus::Active)]));
        assert_eq!(
            session.selected().map(|selected| selected.repos.clone()),
            Some(None)
        );
    }

    #[test]
    fn the_active_repository_survives_reading_the_list_again() {
        let mut session = with_repos(vec![
            repo("a", RepoStatus::Active),
            repo("b", RepoStatus::Active),
        ]);
        assert!(session.activate("b"));

        assert!(session.set_repos(
            "ws",
            vec![repo("b", RepoStatus::Active), repo("c", RepoStatus::Active)]
        ));
        assert_eq!(
            session.active_repo().map(|repo| repo.id.as_str()),
            Some("b")
        );

        assert!(session.set_repos("ws", vec![repo("c", RepoStatus::Active)]));
        assert_eq!(
            session.active_repo(),
            None,
            "what is gone cannot stay active"
        );
    }

    #[test]
    fn restoring_skips_what_cannot_be_shown() {
        let mut session = with_repos(vec![
            repo("a", RepoStatus::Active),
            repo("b", RepoStatus::Active),
        ]);
        assert_eq!(
            session.restore_active(Some("b")).map(|repo| repo.id),
            Some("b".to_string())
        );

        let mut session = with_repos(vec![
            repo("a", RepoStatus::Missing),
            repo("b", RepoStatus::Active),
        ]);
        assert_eq!(
            session.restore_active(Some("a")).map(|repo| repo.id),
            Some("b".to_string()),
            "a missing one gives way"
        );
        assert_eq!(
            session.restore_active(Some("unknown")).map(|repo| repo.id),
            Some("b".to_string()),
            "so does an unknown one"
        );
        assert_eq!(
            session.restore_active(None).map(|repo| repo.id),
            Some("b".to_string())
        );

        let mut session = with_repos(vec![repo("a", RepoStatus::Missing)]);
        assert_eq!(session.restore_active(None), None, "nothing can be shown");
    }

    #[test]
    fn a_repository_whose_path_is_missing_is_not_activated() {
        let mut session = with_repos(vec![
            repo("a", RepoStatus::Active),
            repo("gone", RepoStatus::Missing),
        ]);
        assert!(session.activate("a"));

        assert!(!session.activate("gone"));
        assert!(!session.activate("unknown"));
        assert_eq!(
            session.active_repo().map(|repo| repo.id.as_str()),
            Some("a"),
            "the active one is left alone"
        );
    }

    /// Coming back to a workspace shows the repository that was active when it
    /// was left, not the one the application started on.
    #[test]
    fn the_repository_activated_is_the_one_shown_when_the_workspace_comes_back() {
        let mut session = with_repos(vec![
            repo("a", RepoStatus::Active),
            repo("b", RepoStatus::Active),
        ]);
        assert!(session.restore_active(None).is_some());
        assert!(
            session.keep_active(),
            "the repository restored from nothing is noted"
        );

        assert!(session.activate("b"));
        assert!(
            session.keep_active(),
            "another repository is active: that is noted"
        );

        assert!(session.show("ws"));
        // Coming back, the repositories are read again, and the one noted is restored.
        let noted = session.workspaces()[0].last_active_repo_id.clone();
        assert!(session.set_repos(
            "ws",
            vec![repo("a", RepoStatus::Active), repo("b", RepoStatus::Active)]
        ));
        assert_eq!(
            session.restore_active(noted.as_deref()).map(|repo| repo.id),
            Some("b".to_string()),
            "the workspace shows the repository it was left on"
        );
        assert!(!session.keep_active(), "and that is already what is noted");
    }

    #[test]
    fn nothing_is_active_before_the_repositories_were_read() {
        let mut session = Session::new(vec![workspace("ws")], None);
        assert_eq!(session.restore_active(None), None);
        assert!(!session.activate("r"));
        assert_eq!(session.active_repo(), None);
    }
}
