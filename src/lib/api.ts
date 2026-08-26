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

// ─── Repo commands ───────────────────────────────────────────────────────

export function listRepos(workspaceId: string): Promise<RepoRef[]> {
  return invoke<RepoRef[]>("cmd_list_repos", { workspaceId });
}

export function initRepo(workspaceId: string, path: string): Promise<RepoRef> {
  return invoke<RepoRef>("cmd_init_repo", { workspaceId, path });
}

export function cloneRepo(workspaceId: string, url: string, destPath: string): Promise<RepoRef> {
  return invoke<RepoRef>("cmd_clone_repo", {
    workspaceId,
    url,
    destPath,
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

export function getCommitLog(workspaceId: string, max: number): Promise<CommitSummary[]> {
  return invoke<CommitSummary[]>("cmd_get_commit_log", { workspaceId, max });
}

export function getWorkdirDiff(workspaceId: string): Promise<DiffSummary> {
  return invoke<DiffSummary>("cmd_get_workdir_diff", { workspaceId });
}

export function getCommitDiff(workspaceId: string, commitOid: string): Promise<DiffSummary> {
  return invoke<DiffSummary>("cmd_get_commit_diff", { workspaceId, commitOid });
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

export function checkoutBranch(workspaceId: string, name: string): Promise<void> {
  return invoke<void>("cmd_checkout_branch", { workspaceId, name });
}

export function getAheadBehind(workspaceId: string, branchName: string): Promise<AheadBehind> {
  return invoke<AheadBehind>("cmd_get_ahead_behind", { workspaceId, branchName });
}

export function mergeBranch(workspaceId: string, branchName: string): Promise<MergeResult> {
  return invoke<MergeResult>("cmd_merge_branch", { workspaceId, branchName });
}

export function rebaseBranch(workspaceId: string, upstream: string): Promise<RebaseResult> {
  return invoke<RebaseResult>("cmd_rebase_branch", { workspaceId, upstream });
}

// ─── Working copy ────────────────────────────────────────────────────────────

export type FileStatusKind =
  | "modified"
  | "added"
  | "deleted"
  | "untracked"
  | "renamed"
  | "copied";

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

export function fetchRemote(workspaceId: string, remote?: string): Promise<void> {
  return invoke<void>("cmd_fetch", { workspaceId, remote: remote ?? null });
}

export function pullRemote(workspaceId: string, remote?: string): Promise<void> {
  return invoke<void>("cmd_pull", { workspaceId, remote: remote ?? null });
}

export function pushRemote(workspaceId: string, remote?: string): Promise<void> {
  return invoke<void>("cmd_push", { workspaceId, remote: remote ?? null });
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
