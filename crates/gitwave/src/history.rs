//! The history of a repository: its commits, a page at a time, in a list that
//! draws only the rows on screen.

use std::time::SystemTime;

use gitwave_core::domain::error::AppError;
use gitwave_core::domain::error_codes as codes;
use gitwave_core::domain::history::{CommitPage, CommitRef, CommitRefKind, CommitSummary};
use gpui_kit::component::button::Button;
use gpui_kit::component::label::Label;
use gpui_kit::component::list::{List, ListDelegate, ListItem, ListState};
use gpui_kit::component::{ActiveTheme as _, IndexPath, Sizable as _, h_flex, v_flex};
use gpui_kit::prelude::FluentBuilder as _;
use gpui_kit::{
    AnyElement, AnyWindowHandle, App, AppContext as _, Context, Entity, InteractiveElement as _,
    IntoElement, ParentElement as _, Render, ScrollStrategy, StatefulInteractiveElement as _,
    Styled as _, TestSupportExt as _, WeakEntity, Window, div, px,
};
use rust_i18n::t;

use crate::{app_state, error_text, time_text};

/// The height of a row, as the workbench design sets it for lists.
const ROW_HEIGHT: f32 = 28.;

/// How often the history is read again from the newest commit when it moved
/// on under the cursor, before the user is left to ask.
const READS_AGAIN_ALLOWED: u8 = 2;

/// Which repository's history this is.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Scope {
    pub workspace_id: String,
    pub repo_id: String,
}

/// The commits on screen, and where the page after them is.
#[derive(Debug, Default)]
pub struct Pages {
    pub commits: Vec<CommitSummary>,
    status: Status,
    /// Where the page after the last one starts.
    next_cursor: Option<String>,
    /// The error of the page that did not come.
    failure: Option<AppError>,
    /// How many times the history was read again from the newest commit.
    reads_again: u8,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
enum Status {
    /// The first page is on its way; there is nothing to show yet.
    #[default]
    Loading,
    /// Shown, and the page after the last one can be asked for.
    Ready,
    /// The page after the last one is on its way.
    LoadingMore,
    /// Shown, and there is no page after the last one.
    End,
    /// The history moved on under the cursor. What was shown still is, while
    /// it is read again from the newest commit.
    ReadingAgain,
    /// A page did not come. What was shown before still is. `first` when it
    /// was the first page. Otherwise it was the page after the last one,
    /// which is where `next_cursor` points: asking again reads that page,
    /// and only it.
    Failed { first: bool },
}

impl Pages {
    /// Starts over, for another repository.
    pub fn clear(&mut self) {
        self.commits.clear();
        self.status = Status::Loading;
        self.next_cursor = None;
        self.failure = None;
        self.reads_again = 0;
    }

    /// A page came.
    pub fn push(&mut self, page: CommitPage) {
        // A page that is empty and does not move the cursor on has nothing
        // after it, whatever is said of it: asking on would ask for ever. An
        // empty page whose cursor moves on is a stretch with nothing that
        // matched, which is what a search gives.
        let stalled = page.commits.is_empty() && page.next_cursor == self.next_cursor;
        let more = page.has_more && page.next_cursor.is_some() && !stalled;
        if self.status == Status::ReadingAgain {
            // Read again from the newest commit: what was shown gives way to
            // what was read, it is not added to.
            self.commits = page.commits;
        } else {
            self.commits.extend(page.commits);
        }
        self.next_cursor = page.next_cursor;
        self.failure = None;
        self.status = if more { Status::Ready } else { Status::End };
    }

    /// The page on its way did not come.
    pub fn fail(&mut self, error: AppError) {
        let first = matches!(self.status, Status::Loading | Status::ReadingAgain);
        self.failure = Some(error);
        self.status = Status::Failed { first };
    }

    /// Asks again for the page that did not come.
    pub fn retry(&mut self) -> bool {
        match self.status {
            Status::Failed { first } => {
                if first {
                    self.clear();
                } else {
                    self.status = Status::LoadingMore;
                    self.failure = None;
                }
                true
            }
            _ => false,
        }
    }

    /// Asks for the page after the last one. `false` when there is none, or
    /// when one is already on its way.
    pub fn ask_for_more(&mut self) -> bool {
        if self.status != Status::Ready {
            return false;
        }
        self.status = Status::LoadingMore;
        true
    }

    /// Reads the history again from the newest commit, because the cursor
    /// went stale under it. `false` when that has been done often enough: the
    /// user is left to ask.
    pub fn read_again(&mut self) -> bool {
        if self.reads_again >= READS_AGAIN_ALLOWED {
            return false;
        }
        self.reads_again += 1;
        self.failure = None;
        // With nothing shown there is nothing to keep: it is a first page.
        self.status = if self.commits.is_empty() {
            Status::Loading
        } else {
            Status::ReadingAgain
        };
        self.next_cursor = None;
        true
    }

    /// Whether the first page is on its way.
    pub fn is_loading(&self) -> bool {
        self.status == Status::Loading
    }

    /// Whether the page after the last one can be asked for.
    pub fn has_more(&self) -> bool {
        self.status == Status::Ready
    }

    /// Whether that page is on its way.
    pub fn is_loading_more(&self) -> bool {
        self.status == Status::LoadingMore
    }

    /// Whether the history is being read again from the newest commit.
    pub fn is_reading_again(&self) -> bool {
        self.status == Status::ReadingAgain
    }

    /// The history was shown to its end, and has something to show.
    pub fn is_at_end(&self) -> bool {
        self.status == Status::End && !self.commits.is_empty()
    }

    /// Why a page did not come.
    pub fn failure(&self) -> Option<&AppError> {
        self.failure.as_ref()
    }
}

/// The history of one repository.
pub struct History {
    window: AnyWindowHandle,
    list: Entity<ListState<CommitList>>,
    pages: Pages,
    /// Bumped whenever another repository is shown, so that a page arriving
    /// late for the one before is dropped.
    generation: u64,
    scope: Option<Scope>,
    /// The commit the repository's head is at, to mark it. The shell reads
    /// the head once, for the status bar too, and hands it over.
    head: Option<String>,
}

impl History {
    pub fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let history = cx.weak_entity();
        let list = cx.new(|cx| {
            ListState::new(
                CommitList {
                    commits: Vec::new(),
                    head: None,
                    more: false,
                    loading: false,
                    history,
                },
                window,
                cx,
            )
        });
        Self {
            window: window.window_handle(),
            list,
            pages: Pages::default(),
            generation: 0,
            scope: None,
            head: None,
        }
    }

    /// The commits on screen.
    #[cfg(test)]
    pub fn commits(&self) -> &[CommitSummary] {
        &self.pages.commits
    }

    /// The number of the repository shown now. One more for every repository
    /// shown.
    #[cfg(test)]
    pub(crate) fn generation(&self) -> u64 {
        self.generation
    }

    /// Shows the history of a repository, from its newest commit.
    pub fn show(&mut self, scope: Scope, cx: &mut Context<Self>) {
        self.forget(cx);
        self.scope = Some(scope);
        self.fetch(None, cx);
    }

    /// Shows nothing, for a workspace whose repositories are still being
    /// read. What was shown is gone, and what is on its way for it is dropped
    /// when it comes: the workspace shown now is not the one it was asked for.
    pub fn forget(&mut self, cx: &mut Context<Self>) {
        self.generation += 1;
        self.scope = None;
        self.pages.clear();
        self.head = None;
        self.sync_list(cx);
        self.reset_list(cx);
    }

    /// Tells the history where the head of its repository is, `None` when it
    /// has none or it could not be read.
    pub fn head_is(&mut self, sha: Option<String>, cx: &mut Context<Self>) {
        self.head = sha;
        self.sync_list(cx);
    }

    /// Asks for the page after the last one.
    fn load_more(&mut self, cx: &mut Context<Self>) {
        if !self.pages.ask_for_more() {
            return;
        }
        let cursor = self.pages.next_cursor.clone();
        self.sync_list(cx);
        self.fetch(cursor, cx);
    }

    /// Asks again for the page that did not come.
    fn retry(&mut self, cx: &mut Context<Self>) {
        if !self.pages.retry() {
            return;
        }
        let cursor = self.pages.next_cursor.clone();
        self.sync_list(cx);
        self.fetch(cursor, cx);
    }

    fn fetch(&mut self, cursor: Option<String>, cx: &mut Context<Self>) {
        let Some(scope) = self.scope.clone() else {
            return;
        };
        let generation = self.generation;
        let page = app_state::commit_page(cx, scope.workspace_id, scope.repo_id, cursor);
        // Detached, as the component library's tasks are: it reaches the
        // history through a weak reference, and is harmless once the history
        // is gone.
        cx.spawn(async move |this, cx| {
            let result = page.await;
            let _ = this.update(cx, |history, cx| {
                history.page_arrived(generation, result, cx)
            });
        })
        .detach();
    }

    fn page_arrived(
        &mut self,
        generation: u64,
        result: Result<CommitPage, AppError>,
        cx: &mut Context<Self>,
    ) {
        if generation != self.generation {
            return;
        }
        let was_reading_again = self.pages.is_reading_again();
        match result {
            Ok(page) => self.pages.push(page),
            Err(error) if error.code() == codes::git::HISTORY_CURSOR_EXPIRED => {
                // The history moved on under the cursor. What was shown
                // stays while it is read again from the newest commit, as the
                // previous interface did; after a few times, the user is
                // left to ask.
                if self.pages.read_again() {
                    tracing::info!("the history moved on under the cursor, reading it again");
                    self.sync_list(cx);
                    self.fetch(None, cx);
                    return;
                }
                self.pages.fail(error);
            }
            Err(error) => self.pages.fail(error),
        }
        self.sync_list(cx);
        // What was shown gave way to what was read again from the start.
        if was_reading_again {
            self.reset_list(cx);
        }
    }

    /// Scrolls the list to its end, the way the wheel does. The list then
    /// asks for the page that comes next.
    #[cfg(test)]
    pub(crate) fn scroll_to_end(&mut self, cx: &mut Context<Self>) {
        self.list.update(cx, |list, cx| {
            let rows = list.delegate().items_count(0, cx);
            // Past the end: the list stops at its last row.
            list.scroll_handle()
                .base_handle()
                .set_offset(gpui_kit::point(px(0.), px(-(rows as f32 * ROW_HEIGHT))));
            cx.notify();
        });
    }

    /// The row the list has selected.
    #[cfg(test)]
    pub(crate) fn selected_row(&self, cx: &App) -> Option<usize> {
        self.list.read(cx).selected_index().map(|ix| ix.row)
    }

    /// Takes a page as if it had arrived for the given repository.
    #[cfg(test)]
    pub(crate) fn page_arrived_for_test(
        &mut self,
        generation: u64,
        result: Result<CommitPage, AppError>,
        cx: &mut Context<Self>,
    ) {
        self.page_arrived(generation, result, cx);
    }

    /// Takes the history to the moment the page after the last one is on its
    /// way, without reading it: the test says what came of it. `false` when
    /// there is no such page.
    #[cfg(test)]
    pub(crate) fn page_on_its_way_for_test(&mut self, cx: &mut Context<Self>) -> bool {
        if !self.pages.ask_for_more() {
            return false;
        }
        self.sync_list(cx);
        true
    }

    /// Hands the list what it shows.
    fn sync_list(&mut self, cx: &mut Context<Self>) {
        let commits = self.pages.commits.clone();
        let head = self
            .head
            .as_deref()
            .and_then(|sha| commits.iter().position(|commit| commit.sha == sha));
        let more = self.pages.has_more();
        let loading = self.pages.is_loading();
        self.list.update(cx, |list, cx| {
            let delegate = list.delegate_mut();
            delegate.commits = commits;
            delegate.head = head;
            delegate.more = more;
            delegate.loading = loading;
            cx.notify();
        });
    }

    /// The list keeps the selection and the scroll position of its own. They
    /// belong to what was shown before, so they are reset, once this update
    /// has ended: it takes the window, which cannot be updated from inside
    /// its own update.
    fn reset_list(&mut self, cx: &mut Context<Self>) {
        let (window, list) = (self.window, self.list.clone());
        let app: &mut App = cx;
        app.defer(move |cx| {
            let _ = window.update(cx, |_, window, cx| {
                list.update(cx, |list, cx| {
                    list.set_selected_index(None, window, cx);
                    list.scroll_to_item(IndexPath::new(0), ScrollStrategy::Top, window, cx);
                    cx.notify();
                });
            });
        });
    }
}

impl Render for History {
    fn render(&mut self, _: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let notice: Option<AnyElement> = if let Some(error) = self.pages.failure() {
            Some(
                strip(
                    "history-failure",
                    t!("branches.graph.loadFailed", error = error_text::show(error)).to_string(),
                    true,
                    cx,
                )
                .into_any_element(),
            )
        } else if self.pages.is_reading_again() {
            Some(
                strip(
                    "history-restarting",
                    t!("branches.graph.reloading").to_string(),
                    false,
                    cx,
                )
                .into_any_element(),
            )
        } else {
            None
        };
        let muted = cx.theme().muted_foreground;
        let footer: Option<AnyElement> = if self.pages.is_loading_more() {
            Some(footer_line(t!("branches.graph.loadingOlder"), muted).into_any_element())
        } else if self.pages.is_at_end() {
            Some(
                footer_line(
                    t!(
                        "branches.graph.endOfHistory",
                        total = self.pages.commits.len()
                    ),
                    muted,
                )
                .into_any_element(),
            )
        } else {
            None
        };
        v_flex()
            .id("history")
            .size_full()
            .min_w_0()
            .min_h_0()
            .text_color(cx.theme().foreground)
            .children(notice)
            .child(div().flex_1().min_h_0().child(List::new(&self.list)))
            .children(footer)
    }
}

/// One line above the list when something has to be said: why a page did not
/// come, with a way to ask again; or that the history is being read again.
fn strip(id: &'static str, text: String, retry: bool, cx: &mut Context<History>) -> AnyElement {
    h_flex()
        .id(id)
        .test_support()
        .aria_label(text.clone())
        .h(px(28.))
        .flex_shrink_0()
        .px_3()
        .gap_2()
        .text_xs()
        .text_color(if retry {
            cx.theme().danger
        } else {
            cx.theme().muted_foreground
        })
        .child(Label::new(text))
        .when(retry, |this| {
            this.child(
                Button::new("history-retry")
                    .small()
                    .label(t!("common.retry"))
                    .on_click(cx.listener(|this, _, _, cx| this.retry(cx))),
            )
        })
        .into_any_element()
}

fn footer_line(text: impl Into<gpui_kit::SharedString>, muted: gpui_kit::Hsla) -> impl IntoElement {
    h_flex()
        .h(px(24.))
        .flex_shrink_0()
        .justify_center()
        .text_xs()
        .text_color(muted)
        .child(Label::new(text))
}

/// What the list is given: the commits, where the head is, and how to ask for
/// the page that comes next.
struct CommitList {
    commits: Vec<CommitSummary>,
    /// The commit the repository's head is at.
    head: Option<usize>,
    /// Whether the list is to ask for the page that comes next.
    more: bool,
    /// Whether the first page is on its way.
    loading: bool,
    /// The history that owns the list, asked for that page.
    history: WeakEntity<History>,
}

impl ListDelegate for CommitList {
    type Item = ListItem;

    fn items_count(&self, _: usize, _: &App) -> usize {
        self.commits.len()
    }

    fn render_item(
        &mut self,
        ix: IndexPath,
        _: &mut Window,
        cx: &mut Context<ListState<Self>>,
    ) -> Option<Self::Item> {
        let commit = self.commits.get(ix.row)?;
        // The list marks the selected row itself.
        Some(
            ListItem::new(("commit", ix.row))
                .h(px(ROW_HEIGHT))
                .accessibility_label(row_label(commit, Some(ix.row) == self.head))
                .child(row(
                    commit,
                    Some(ix.row) == self.head,
                    SystemTime::now(),
                    cx,
                )),
        )
    }

    fn render_empty(
        &mut self,
        _: &mut Window,
        _: &mut Context<ListState<Self>>,
    ) -> impl IntoElement {
        centered(t!("branches.graph.empty"))
    }

    fn loading(&self, _: &App) -> bool {
        self.loading
    }

    fn render_loading(
        &mut self,
        _: &mut Window,
        _: &mut Context<ListState<Self>>,
    ) -> impl IntoElement {
        centered(t!("branches.graph.loading"))
    }

    fn set_selected_index(
        &mut self,
        _: Option<IndexPath>,
        _: &mut Window,
        _: &mut Context<ListState<Self>>,
    ) {
        // The list keeps the selection; there is nothing of the history's to
        // do with it as yet.
    }

    fn has_more(&self, _: &App) -> bool {
        self.more
    }

    fn load_more(&mut self, window: &mut Window, cx: &mut Context<ListState<Self>>) {
        let history = self.history.clone();
        // The history cannot be updated from inside the list's own update,
        // nor when the update's deferred callbacks run: the list is still
        // being drawn then. A task runs once the scheduler comes round.
        cx.spawn_in(window, async move |_, cx| {
            let _ = history.update(cx, |history, cx| history.load_more(cx));
        })
        .detach();
    }
}

fn centered(text: impl Into<gpui_kit::SharedString>) -> impl IntoElement {
    h_flex()
        .size_full()
        .justify_center()
        .child(Label::new(text))
}

/// What a screen reader reads for a row: whether it is the head, its
/// references, the subject, the short hash, the author and how long ago.
fn row_label(commit: &CommitSummary, is_head: bool) -> gpui_kit::SharedString {
    let mut label = String::new();
    if is_head {
        label.push_str("HEAD ");
    }
    for reference in &commit.refs {
        label.push_str(&reference.name);
        label.push(' ');
    }
    let at = chrono::DateTime::from_timestamp(commit.time, 0)
        .map(|time| {
            time.with_timezone(&chrono::Local)
                .format("%Y-%m-%d %H:%M")
                .to_string()
        })
        .unwrap_or_default();
    format!(
        "{label}{} {} {} {at}",
        commit.message_summary,
        short_sha(&commit.sha),
        commit.author
    )
    .into()
}

/// One commit: the lane the graph will use, its main reference, its subject,
/// then the hash, the author and when it was made, each in a column of its
/// own so that they line up. The columns are as wide as their longest text
/// at the interface's largest font, and no text wraps: a row is one line.
fn row(commit: &CommitSummary, is_head: bool, now: SystemTime, cx: &App) -> AnyElement {
    let theme = cx.theme();
    let main = primary_ref(commit);
    let others = main.map_or(0, |_| commit.refs.len() - 1);
    let label = |text: String| {
        div()
            .px_1()
            .rounded_sm()
            .text_xs()
            .max_w(px(160.))
            .truncate()
            .border_1()
            .border_color(theme.border)
            .child(text)
    };
    h_flex()
        .w_full()
        .items_center()
        .gap_2()
        .pr_2()
        .text_color(theme.foreground)
        // The graph takes this lane; its lines come with the graph.
        .child(div().w(px(18.)).flex_shrink_0())
        .when(is_head, |this| {
            this.child(
                div()
                    .id("history-head")
                    .test_support()
                    .px_1()
                    .rounded_sm()
                    .text_xs()
                    .border_1()
                    .border_color(theme.border)
                    .child("HEAD"),
            )
        })
        .when_some(main, |this, main| this.child(label(main.name.clone())))
        .when(others > 0, |this| {
            this.child(
                div()
                    .text_xs()
                    .flex_shrink_0()
                    .text_color(theme.muted_foreground)
                    .child(format!("+{others}")),
            )
        })
        .child(
            div()
                .flex_1()
                .min_w_0()
                .truncate()
                .child(commit.message_summary.clone()),
        )
        .child(
            div()
                .flex_shrink_0()
                .w(px(56.))
                .text_xs()
                .text_color(theme.muted_foreground)
                .font_family(theme.mono_font_family.clone())
                .truncate()
                .child(short_sha(&commit.sha)),
        )
        .child(
            div()
                .flex_shrink_0()
                .w(px(110.))
                .truncate()
                .text_color(theme.muted_foreground)
                .child(commit.author.clone()),
        )
        .child(
            div()
                .flex_shrink_0()
                .w(px(96.))
                .text_right()
                .text_color(theme.muted_foreground)
                // A date is ten characters wide: it must not wrap inside a row.
                .truncate()
                .child(time_text::ago(now, commit.time)),
        )
        .into_any_element()
}

fn short_sha(sha: &str) -> String {
    sha.chars().take(7).collect()
}

/// The reference a commit is known by: a tag first, then a local branch,
/// then a remote one.
fn primary_ref(commit: &CommitSummary) -> Option<&CommitRef> {
    [
        CommitRefKind::Tag,
        CommitRefKind::LocalBranch,
        CommitRefKind::RemoteBranch,
    ]
    .into_iter()
    .find_map(|kind| commit.refs.iter().find(|reference| reference.kind == kind))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn commit(sha: &str) -> CommitSummary {
        CommitSummary {
            sha: sha.to_string(),
            author: "Ada".to_string(),
            author_email: "ada@example.com".to_string(),
            time: 0,
            message_summary: sha.to_string(),
            lane: 0,
            parents: vec![],
            refs: vec![],
        }
    }

    fn page(shas: &[&str], has_more: bool) -> CommitPage {
        CommitPage {
            commits: shas.iter().map(|sha| commit(sha)).collect(),
            next_cursor: has_more.then(|| "next".to_string()),
            has_more,
            scanned: 0,
            snapshot_size: 0,
            snapshot_truncated: false,
        }
    }

    /// A page like the ones a search gives: empty, and a cursor that moved on.
    fn empty_page_with(cursor: &str) -> CommitPage {
        CommitPage {
            commits: vec![],
            next_cursor: Some(cursor.to_string()),
            has_more: true,
            scanned: 0,
            snapshot_size: 0,
            snapshot_truncated: false,
        }
    }

    fn shas(pages: &Pages) -> Vec<&str> {
        pages
            .commits
            .iter()
            .map(|commit| commit.sha.as_str())
            .collect()
    }

    #[test]
    fn pages_follow_each_other_until_the_end() {
        let mut pages = Pages::default();
        assert!(pages.is_loading());

        pages.push(page(&["c", "b"], true));
        assert_eq!(shas(&pages), ["c", "b"]);
        assert!(pages.has_more(), "a page is said to come next");

        assert!(pages.ask_for_more());
        assert!(pages.is_loading_more());
        assert!(!pages.ask_for_more(), "one page at a time");

        pages.push(page(&["a"], false));
        assert_eq!(shas(&pages), ["c", "b", "a"]);
        assert!(pages.is_at_end());
        assert!(!pages.ask_for_more(), "there is no page after the last one");
    }

    #[test]
    fn an_empty_page_that_does_not_move_on_ends_the_history() {
        let mut pages = Pages::default();
        pages.push(page(&["c"], true));
        pages.ask_for_more();

        pages.push(page(&[], true));

        assert!(pages.is_at_end(), "asking on would ask for ever");
    }

    #[test]
    fn an_empty_page_that_moves_on_is_not_the_end() {
        let mut pages = Pages::default();
        pages.push(page(&["c"], true));
        pages.ask_for_more();

        // A stretch of the history with nothing that matched.
        pages.push(empty_page_with("further"));

        assert_eq!(shas(&pages), ["c"], "what was shown stays");
        assert!(
            pages.has_more(),
            "and the page after it is asked for all the same"
        );
    }

    #[test]
    fn a_page_that_does_not_arrive_keeps_what_was_shown() {
        let mut pages = Pages::default();
        pages.push(page(&["c", "b"], true));
        pages.ask_for_more();
        pages.fail(AppError::unknown("some.code", "gone"));

        assert_eq!(
            pages.failure().map(AppError::message).as_deref(),
            Some("unknown error: gone")
        );
        assert_eq!(shas(&pages), ["c", "b"], "what was shown still is");
        assert!(!pages.has_more(), "and nothing else is asked for by itself");

        assert!(pages.retry());
        assert!(
            pages.is_loading_more(),
            "the page that did not come is asked for again"
        );
        assert!(
            pages.failure().is_none(),
            "and the reason is no longer shown"
        );
        pages.push(page(&["a"], false));
        assert_eq!(shas(&pages), ["c", "b", "a"]);
        assert!(!pages.retry(), "nothing to ask again for");
    }

    #[test]
    fn a_first_page_that_does_not_arrive_is_asked_for_again_from_the_start() {
        let mut pages = Pages::default();
        pages.fail(AppError::unknown("some.code", "no repository there"));

        assert!(pages.retry());
        assert!(pages.is_loading());
        assert!(pages.commits.is_empty());
    }

    #[test]
    fn starting_over_forgets_the_repository_before() {
        let mut pages = Pages::default();
        pages.push(page(&["c"], true));

        pages.clear();

        assert!(pages.is_loading());
        assert!(pages.commits.is_empty());
        pages.push(page(&["z"], false));
        assert_eq!(shas(&pages), ["z"], "only the repository shown now");
    }

    #[test]
    fn a_cursor_that_went_stale_reads_again_from_the_newest_commit_keeping_what_was_shown() {
        let mut pages = Pages::default();
        pages.push(page(&["c", "b"], true));
        pages.ask_for_more();

        assert!(pages.read_again());
        assert!(pages.is_reading_again());
        assert_eq!(
            shas(&pages),
            ["c", "b"],
            "what was shown stays while it is read again"
        );
        assert!(
            !pages.has_more(),
            "and nothing more is asked for the stale cursor"
        );

        pages.push(page(&["d", "c", "b", "a"], true));
        assert_eq!(
            shas(&pages),
            ["d", "c", "b", "a"],
            "what was read again replaces what was shown"
        );
        assert!(pages.has_more());
    }

    #[test]
    fn the_history_is_read_again_only_so_often() {
        let mut pages = Pages::default();
        pages.push(page(&["c"], true));

        assert!(pages.read_again());
        pages.push(page(&["c"], true));
        assert!(pages.read_again());
        pages.push(page(&["c"], true));
        assert!(
            !pages.read_again(),
            "the history moves on faster than it can be read"
        );

        pages.clear();
        assert!(
            pages.read_again(),
            "another repository starts with a clean slate"
        );
    }

    #[test]
    fn reading_an_empty_history_again_is_a_first_page() {
        let mut pages = Pages::default();

        assert!(pages.read_again());
        assert!(
            pages.is_loading(),
            "with nothing to keep, there is nothing to keep showing"
        );
        assert!(!pages.is_reading_again());
    }

    #[test]
    fn the_main_reference_is_the_tag_then_the_local_then_the_remote_branch() {
        let reference = |name: &str, kind: CommitRefKind| CommitRef {
            name: name.to_string(),
            kind,
        };
        let mut commit = commit("a");
        commit.refs = vec![reference("origin/main", CommitRefKind::RemoteBranch)];
        assert_eq!(
            primary_ref(&commit).map(|r| r.name.as_str()),
            Some("origin/main")
        );
        commit
            .refs
            .push(reference("main", CommitRefKind::LocalBranch));
        assert_eq!(primary_ref(&commit).map(|r| r.name.as_str()), Some("main"));
        commit.refs.push(reference("v1", CommitRefKind::Tag));
        assert_eq!(primary_ref(&commit).map(|r| r.name.as_str()), Some("v1"));
        assert_eq!(short_sha("0123456789abcdef"), "0123456");
    }
}
