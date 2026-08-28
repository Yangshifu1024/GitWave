// Typed wrappers around Tauri commands. Mirrors the Rust types in
// src-tauri/src/domain/workspace.rs, ssh/keys.rs, and error.rs.
//
// Errors thrown by invoke follow `AppError`'s serialized shape:
//   { category: string, message: string, trace_id: string }
//
// Tauri command argument names are camelCase by default (Tauri converts
// `workspace_id` Rust field to `workspaceId` JS key).

import { invoke } from "@tauri-apps/api/core";

export interface PromptTemplates {
  commit: string | null;
  conflict: string | null;
  pr: string | null;
}

export interface WorkspaceSettings {
  ai_provider: string | null;
  ai_model?: string | null;
  ai_base_url?: string | null;
  /** PM 1.6 offline mode: refuse cloud AI calls (Ollama keeps working). */
  ai_offline?: boolean;
  prompt_templates: PromptTemplates;
  commit_convention: string | null;
  theme_override: string | null;
  key_binding_profile: string | null;
}

export interface RepoRef {
  id: string;
  workspace_id: string;
  path: string;
  nickname: string | null;
  settings_override: WorkspaceSettings | null;
  status: "active" | "missing";
  missing_since: number | null;
  added_at: number;
}

export interface Workspace {
  id: string;
  name: string;
  repos: RepoRef[];
  settings: WorkspaceSettings;
  last_active_repo_id: string | null;
  created_at: number;
  updated_at: number;
}

export interface WorkspaceSummary {
  id: string;
  name: string;
  last_active_repo_id: string | null;
  updated_at: number;
}

export interface AppError {
  category: string;
  message: string;
  trace_id: string;
}

export interface SshKey {
  path: string;
  fingerprint: string;
  comment: string;
}

export interface SshTestResult {
  host: string;
  user: string;
  success: boolean;
  message: string;
}

export function formatAppError(err: unknown): string {
  if (err && typeof err === "object") {
    const e = err as Partial<AppError>;
    if (typeof e.message === "string" && typeof e.category === "string") {
      return `${e.category}: ${e.message}`;
    }
  }
  return String(err);
}

export function getAppVersion(): Promise<string> {
  return invoke<string>("get_app_version");
}

/** Open the platform state directory (SQLite home) in the OS file manager. */
export function openDataDir(): Promise<void> {
  return invoke<void>("open_data_dir");
}

// ─── Workspace commands ───────────────────────────────────────────────────

export function listWorkspaces(): Promise<WorkspaceSummary[]> {
  return invoke<WorkspaceSummary[]>("cmd_list_workspaces");
}

export function createWorkspace(name: string): Promise<Workspace> {
  return invoke<Workspace>("cmd_create_workspace", { name });
}

export function renameWorkspace(id: string, newName: string): Promise<void> {
  return invoke<void>("cmd_rename_workspace", { id, newName });
}

export function deleteWorkspace(id: string): Promise<void> {
  return invoke<void>("cmd_delete_workspace", { id });
}

export function setActiveRepo(workspaceId: string, repoId: string | null): Promise<void> {
  return invoke<void>("cmd_set_active_repo", { workspaceId, repoId });
}

export function getWorkspace(id: string): Promise<Workspace> {
  return invoke<Workspace>("cmd_get_workspace", { id });
}

export function updateWorkspaceSettings(id: string, settings: WorkspaceSettings): Promise<void> {
  return invoke<void>("cmd_update_workspace_settings", { id, settings });
}

export interface AiKeyStatus {
  provider: string;
  has_key: boolean;
}

export function setAiApiKey(workspaceId: string, provider: string, apiKey: string): Promise<void> {
  return invoke<void>("cmd_set_ai_api_key", { workspaceId, provider, apiKey });
}

export function clearAiApiKey(workspaceId: string, provider: string): Promise<void> {
  return invoke<void>("cmd_clear_ai_api_key", { workspaceId, provider });
}

export function getAiKeyStatus(workspaceId: string, provider: string): Promise<AiKeyStatus> {
  return invoke<AiKeyStatus>("cmd_get_ai_key_status", { workspaceId, provider });
}

export function probeOllama(baseUrl?: string): Promise<string[]> {
  return invoke<string[]>("cmd_probe_ollama", { baseUrl: baseUrl ?? null });
}

export function generateCommitMessage(workspaceId: string): Promise<string> {
  return invoke<string>("cmd_generate_commit_message", { workspaceId });
}

// ─── Repo commands ───────────────────────────────────────────────────────

export function listRepos(workspaceId: string): Promise<RepoRef[]> {
  return invoke<RepoRef[]>("cmd_list_repos", { workspaceId });
}

export function initRepo(workspaceId: string, path: string): Promise<RepoRef> {
  return invoke<RepoRef>("cmd_init_repo", { workspaceId, path });
}

export interface CloneProgress {
  receivedObjects: number;
  totalObjects: number;
  indexedDeltas: number;
  totalDeltas: number;
  receivedBytes: number;
}

export type SyncOperation = "fetch" | "pull" | "push";

export interface SyncProgress {
  operation: SyncOperation;
  receivedObjects: number;
  totalObjects: number;
  receivedBytes: number;
}

export function cloneRepo(
  workspaceId: string,
  url: string,
  destPath: string,
  replaceDest = false,
): Promise<RepoRef> {
  return invoke<RepoRef>("cmd_clone_repo", {
    workspaceId,
    url,
    destPath,
    replaceDest,
  });
}

export function addLocalRepo(workspaceId: string, path: string): Promise<RepoRef> {
  return invoke<RepoRef>("cmd_add_local_repo", { workspaceId, path });
}

export function removeRepo(workspaceId: string, repoId: string): Promise<void> {
  return invoke<void>("cmd_remove_repo", { workspaceId, repoId });
}

export function relinkRepo(workspaceId: string, repoId: string, newPath: string): Promise<void> {
  return invoke<void>("cmd_relink_repo", {
    workspaceId,
    repoId,
    newPath,
  });
}

// ─── SSH commands ─────────────────────────────────────────────────────────

export function listSshKeys(): Promise<SshKey[]> {
  return invoke<SshKey[]>("cmd_list_ssh_keys");
}

export function addSshKey(path: string): Promise<void> {
  return invoke<void>("cmd_add_ssh_key", { path });
}

export function deleteSshKey(path: string): Promise<void> {
  return invoke<void>("cmd_delete_ssh_key", { path });
}

export function testSshConnection(host: string, user: string): Promise<SshTestResult> {
  return invoke<SshTestResult>("cmd_test_ssh_connection", { host, user });
}

// ─── History / Diff / Blame / Branch commands (Sprint 3) ─────────────────

export type FileStatus = "added" | "modified" | "deleted" | "renamed" | "copied" | "untracked";

export interface FileSummary {
  path: string;
  old_path: string | null;
  kind: FileStatus;
  additions: number;
  deletions: number;
}

export interface CommitRef {
  name: string;
  kind: "local_branch" | "remote_branch" | "tag" | "head";
}

export interface CommitSummary {
  sha: string;
  author: string;
  author_email: string;
  time: number; // unix epoch seconds
  message_summary: string;
  lane: number;
  parents: string[];
  refs: CommitRef[];
}

export interface CommitDetails {
  sha: string;
  author: string;
  author_email: string;
  time: number;
  message_full: string;
  parents: string[];
  files: FileSummary[];
}

export type DiffLineKind = "added" | "removed" | "context";

export interface DiffLine {
  kind: DiffLineKind;
  content: string;
  old_line_no: number | null;
  new_line_no: number | null;
}

export interface DiffHunk {
  old_start: number;
  old_lines: number;
  new_start: number;
  new_lines: number;
  lines: DiffLine[];
}

export interface FileDiff {
  path: string;
  old_sha: string | null;
  new_sha: string | null;
  additions: number;
  deletions: number;
  hunks: DiffHunk[];
  /** Working-copy only: true = index vs HEAD, false = worktree vs index. */
  staged?: boolean | null;
}

export interface DiffSummary {
  files: FileDiff[];
  total_additions: number;
  total_deletions: number;
}

export interface BlameLine {
  line_no: number;
  sha: string;
  author: string;
  author_email: string;
  time: number;
  content: string;
}

export type BranchKind = "local" | "remote";

export interface BranchInfo {
  name: string;
  kind: BranchKind;
  is_current: boolean;
  upstream: string | null;
  ahead: number;
  behind: number;
  last_commit_sha: string;
  last_commit_time: number;
}

export interface AheadBehind {
  ahead: number;
  behind: number;
}

export type MergeKind = "fast_forward" | "three_way" | "already_up_to_date";

export interface MergeResult {
  kind: MergeKind;
  conflicts: string[];
  new_head: string;
}

export type RebaseKind = "already_up_to_date" | "clean" | "conflicts";

export interface RebaseResult {
  kind: RebaseKind;
  conflicts: string[];
  new_head: string | null;
}

// ─── API wrappers ──────────────────────────────────────────────────────────

export function getCommitLog(
  workspaceId: string,
  max: number,
  filter?: string | null,
): Promise<CommitSummary[]> {
  return invoke<CommitSummary[]>("cmd_get_commit_log", {
    workspaceId,
    max,
    filter: filter || null,
  });
}

export function getWorkdirDiff(workspaceId: string): Promise<DiffSummary> {
  return invoke<DiffSummary>("cmd_get_workdir_diff", { workspaceId });
}

export function getCommitDiff(workspaceId: string, commitOid: string): Promise<DiffSummary> {
  return invoke<DiffSummary>("cmd_get_commit_diff", { workspaceId, commitOid });
}

export function getCommitDetails(workspaceId: string, commitOid: string): Promise<CommitDetails> {
  return invoke<CommitDetails>("cmd_get_commit_details", { workspaceId, commitOid });
}

export function getFileDiff(
  workspaceId: string,
  fromOid: string,
  toOid: string,
): Promise<FileDiff[]> {
  return invoke<FileDiff[]>("cmd_get_file_diff", { workspaceId, fromOid, toOid });
}

export function getBlame(workspaceId: string, path: string): Promise<BlameLine[]> {
  return invoke<BlameLine[]>("cmd_get_blame", { workspaceId, path });
}

export function getBranches(workspaceId: string): Promise<BranchInfo[]> {
  return invoke<BranchInfo[]>("cmd_get_branches", { workspaceId });
}

export function createBranch(
  workspaceId: string,
  name: string,
  fromSha: string,
): Promise<BranchInfo> {
  return invoke<BranchInfo>("cmd_create_branch", { workspaceId, name, fromSha });
}

export function deleteBranch(workspaceId: string, name: string): Promise<void> {
  return invoke<void>("cmd_delete_branch", { workspaceId, name });
}

export function checkoutBranch(workspaceId: string, name: string, force = false): Promise<void> {
  return invoke<void>("cmd_checkout_branch", { workspaceId, name, force });
}

export function getAheadBehind(workspaceId: string, branchName: string): Promise<AheadBehind> {
  return invoke<AheadBehind>("cmd_get_ahead_behind", { workspaceId, branchName });
}

export function mergeBranch(
  workspaceId: string,
  branchName: string,
  noFf = false,
): Promise<MergeResult> {
  return invoke<MergeResult>("cmd_merge_branch", { workspaceId, branchName, noFf });
}

export interface MergePreview {
  up_to_date: boolean;
  fast_forward: boolean;
  conflicts: string[];
}

export function mergePreview(workspaceId: string, branchName: string): Promise<MergePreview> {
  return invoke<MergePreview>("cmd_merge_preview", { workspaceId, branchName });
}

export function rebaseBranch(workspaceId: string, upstream: string): Promise<RebaseResult> {
  return invoke<RebaseResult>("cmd_rebase_branch", { workspaceId, upstream });
}

export function revertCommit(workspaceId: string, commitOid: string): Promise<string> {
  return invoke<string>("cmd_revert_commit", { workspaceId, commitOid });
}

export function cherryPickCommit(workspaceId: string, commitOid: string): Promise<string> {
  return invoke<string>("cmd_cherry_pick_commit", { workspaceId, commitOid });
}

// ─── Tags (S3) ───────────────────────────────────────────────────────────────

export interface TagInfo {
  name: string;
  sha: string;
  annotation: string | null;
}

export function listTags(workspaceId: string): Promise<TagInfo[]> {
  return invoke<TagInfo[]>("cmd_list_tags", { workspaceId });
}

export function createTag(
  workspaceId: string,
  name: string,
  targetOid: string | null,
  message: string | null,
): Promise<string> {
  return invoke<string>("cmd_create_tag", { workspaceId, name, targetOid, message });
}

export function deleteTag(workspaceId: string, name: string): Promise<void> {
  return invoke<void>("cmd_delete_tag", { workspaceId, name });
}

// ─── Submodules (S1) ─────────────────────────────────────────────────────────

export interface SubmoduleInfo {
  name: string;
  path: string;
  url: string | null;
  initialized: boolean;
  head_sha: string | null;
}

export function listSubmodules(workspaceId: string): Promise<SubmoduleInfo[]> {
  return invoke<SubmoduleInfo[]>("cmd_list_submodules", { workspaceId });
}

export function initSubmodule(workspaceId: string, name: string): Promise<void> {
  return invoke<void>("cmd_init_submodule", { workspaceId, name });
}

export function updateSubmodule(workspaceId: string, name: string): Promise<void> {
  return invoke<void>("cmd_update_submodule", { workspaceId, name });
}

// --- .gitignore editor (S2) -------------------------------------------------

export function getGitignore(workspaceId: string): Promise<string> {
  return invoke<string>("cmd_get_gitignore", { workspaceId });
}

export function writeGitignore(workspaceId: string, content: string): Promise<void> {
  return invoke<void>("cmd_write_gitignore", { workspaceId, content });
}

// --- Workspace transfer (S6, .gitwave-workspace.json) ------------------------

export function exportWorkspace(workspaceId: string, destPath: string): Promise<string> {
  return invoke<string>("cmd_export_workspace", { workspaceId, destPath });
}

export function importWorkspace(
  srcPath: string,
  newName: string | null,
): Promise<WorkspaceSummary> {
  return invoke<WorkspaceSummary>("cmd_import_workspace", { srcPath, newName });
}

// ─── Interactive rebase ──────────────────────────────────────────────────────

export type InteractiveRebaseAction = "pick" | "reword" | "edit" | "squash" | "fixup" | "drop";

export interface InteractiveRebaseTodo {
  oid: string;
  summary: string;
  action: InteractiveRebaseAction;
  message: string | null;
}

export type InteractiveRebaseKind =
  "clean" | "already_up_to_date" | "conflicts" | "paused_for_edit";

export interface InteractiveRebaseResult {
  kind: InteractiveRebaseKind;
  conflicts: string[];
  new_head: string | null;
}

export function planInteractiveRebase(
  workspaceId: string,
  upstream: string,
): Promise<InteractiveRebaseTodo[]> {
  return invoke<InteractiveRebaseTodo[]>("cmd_plan_interactive_rebase", {
    workspaceId,
    upstream,
  });
}

export function executeInteractiveRebase(
  workspaceId: string,
  upstream: string,
  todos: InteractiveRebaseTodo[],
): Promise<InteractiveRebaseResult> {
  return invoke<InteractiveRebaseResult>("cmd_execute_interactive_rebase", {
    workspaceId,
    upstream,
    todos,
  });
}

export function continueInteractiveRebase(workspaceId: string): Promise<InteractiveRebaseResult> {
  return invoke<InteractiveRebaseResult>("cmd_continue_interactive_rebase", { workspaceId });
}

export function abortInteractiveRebasePause(workspaceId: string): Promise<void> {
  return invoke<void>("cmd_abort_interactive_rebase_pause", { workspaceId });
}

export function interactiveRebasePaused(workspaceId: string): Promise<boolean> {
  return invoke<boolean>("cmd_interactive_rebase_paused", { workspaceId });
}

// ─── Conflicts ───────────────────────────────────────────────────────────────

export interface ConflictFile {
  path: string;
  has_ours: boolean;
  has_theirs: boolean;
  has_base: boolean;
}

export interface ConflictSides {
  path: string;
  ours: string | null;
  theirs: string | null;
  base: string | null;
  working: string | null;
}

export function listConflicts(workspaceId: string): Promise<ConflictFile[]> {
  return invoke<ConflictFile[]>("cmd_list_conflicts", { workspaceId });
}

export function getConflictSides(workspaceId: string, path: string): Promise<ConflictSides> {
  return invoke<ConflictSides>("cmd_get_conflict_sides", { workspaceId, path });
}

export function resolveConflict(workspaceId: string, path: string, content: string): Promise<void> {
  return invoke<void>("cmd_resolve_conflict", { workspaceId, path, content });
}

export function abortMerge(workspaceId: string): Promise<void> {
  return invoke<void>("cmd_abort_merge", { workspaceId });
}

export function mergeInProgress(workspaceId: string): Promise<boolean> {
  return invoke<boolean>("cmd_merge_in_progress", { workspaceId });
}

export function explainConflict(workspaceId: string, path: string): Promise<string> {
  return invoke<string>("cmd_explain_conflict", { workspaceId, path });
}

// ─── Working copy ────────────────────────────────────────────────────────────

export type FileStatusKind = "modified" | "added" | "deleted" | "untracked" | "renamed" | "copied";

export interface FileChange {
  path: string;
  old_path?: string | null;
  kind: FileStatusKind;
  staged: boolean;
  additions: number;
  deletions: number;
}

export interface WorkingCopy {
  repo_id: string;
  branch: string;
  upstream: string | null;
  sha: string;
  ahead: number;
  behind: number;
  files: FileChange[];
}

export function getWorkingCopy(workspaceId: string): Promise<WorkingCopy> {
  return invoke<WorkingCopy>("cmd_get_working_copy", { workspaceId });
}

export function stageFiles(workspaceId: string, paths: string[]): Promise<void> {
  return invoke<void>("cmd_stage_files", { workspaceId, paths });
}

export function unstageFiles(workspaceId: string, paths: string[]): Promise<void> {
  return invoke<void>("cmd_unstage_files", { workspaceId, paths });
}

export function stageAll(workspaceId: string): Promise<void> {
  return invoke<void>("cmd_stage_all", { workspaceId });
}

export function commit(workspaceId: string, message: string): Promise<string> {
  return invoke<string>("cmd_commit", { workspaceId, message });
}

export function discardChanges(workspaceId: string, paths: string[]): Promise<void> {
  return invoke<void>("cmd_discard_changes", { workspaceId, paths });
}

export function ignorePath(workspaceId: string, pattern: string): Promise<void> {
  return invoke<void>("cmd_ignore_path", { workspaceId, pattern });
}

export function fetchRemote(workspaceId: string, remote?: string): Promise<void> {
  return invoke<void>("cmd_fetch", { workspaceId, remote: remote ?? null });
}

export interface PullOptions {
  remote?: string;
  /** Remote-tracking branch short name (e.g. `main`); defaults to the configured upstream. */
  branch?: string;
  rebase?: boolean;
  stash?: boolean;
}

export function pullRemote(workspaceId: string, options?: PullOptions): Promise<void> {
  return invoke<void>("cmd_pull", {
    workspaceId,
    remote: options?.remote ?? null,
    branch: options?.branch ?? null,
    rebase: options?.rebase ?? false,
    stash: options?.stash ?? false,
  });
}

export function listRemotes(workspaceId: string): Promise<string[]> {
  return invoke<string[]>("cmd_list_remotes", { workspaceId });
}

export function deleteRemoteBranch(
  workspaceId: string,
  remote: string,
  branch: string,
): Promise<void> {
  return invoke<void>("cmd_delete_remote_branch", { workspaceId, remote, branch });
}

export interface PushOptions {
  remote?: string;
  tags?: boolean;
  force?: boolean;
}

export function pushRemote(workspaceId: string, options?: PushOptions): Promise<void> {
  return invoke<void>("cmd_push", {
    workspaceId,
    remote: options?.remote ?? null,
    tags: options?.tags ?? false,
    force: options?.force ?? false,
  });
}

// ─── Stash ───────────────────────────────────────────────────────────────────

export interface StashEntry {
  index: number;
  message: string;
  oid: string;
}

export function listStashes(workspaceId: string): Promise<StashEntry[]> {
  return invoke<StashEntry[]>("cmd_list_stashes", { workspaceId });
}

export function saveStash(workspaceId: string, message?: string): Promise<string> {
  return invoke<string>("cmd_save_stash", {
    workspaceId,
    message: message ?? null,
  });
}

export function applyStash(workspaceId: string, index: number): Promise<void> {
  return invoke<void>("cmd_apply_stash", { workspaceId, index });
}

export function popStash(workspaceId: string, index: number): Promise<void> {
  return invoke<void>("cmd_pop_stash", { workspaceId, index });
}

export function dropStash(workspaceId: string, index: number): Promise<void> {
  return invoke<void>("cmd_drop_stash", { workspaceId, index });
}

export function getStashDiff(workspaceId: string, oid: string): Promise<DiffSummary> {
  return invoke<DiffSummary>("cmd_get_stash_diff", { workspaceId, oid });
}

// ─── Worktree ────────────────────────────────────────────────────────────────

export interface WorktreeInfo {
  name: string;
  path: string;
  is_main: boolean;
  is_locked: boolean;
  branch: string | null;
}

export function listWorktrees(workspaceId: string): Promise<WorktreeInfo[]> {
  return invoke<WorktreeInfo[]>("cmd_list_worktrees", { workspaceId });
}

export function addWorktree(
  workspaceId: string,
  name: string,
  path: string,
  branch: string,
  createBranch: boolean,
): Promise<WorktreeInfo> {
  return invoke<WorktreeInfo>("cmd_add_worktree", {
    workspaceId,
    name,
    path,
    branch,
    createBranch,
  });
}

export function removeWorktree(workspaceId: string, name: string): Promise<void> {
  return invoke<void>("cmd_remove_worktree", { workspaceId, name });
}
