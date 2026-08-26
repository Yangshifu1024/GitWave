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

export function setActiveRepo(
  workspaceId: string,
  repoId: string | null,
): Promise<void> {
  return invoke<void>("cmd_set_active_repo", { workspaceId, repoId });
}

// ─── Repo commands ───────────────────────────────────────────────────────

export function listRepos(workspaceId: string): Promise<RepoRef[]> {
  return invoke<RepoRef[]>("cmd_list_repos", { workspaceId });
}

export function initRepo(workspaceId: string, path: string): Promise<RepoRef> {
  return invoke<RepoRef>("cmd_init_repo", { workspaceId, path });
}

export function cloneRepo(
  workspaceId: string,
  url: string,
  destPath: string,
): Promise<RepoRef> {
  return invoke<RepoRef>("cmd_clone_repo", {
    workspaceId,
    url,
    destPath,
  });
}

export function addLocalRepo(
  workspaceId: string,
  path: string,
): Promise<RepoRef> {
  return invoke<RepoRef>("cmd_add_local_repo", { workspaceId, path });
}

export function removeRepo(workspaceId: string, repoId: string): Promise<void> {
  return invoke<void>("cmd_remove_repo", { workspaceId, repoId });
}

export function relinkRepo(
  workspaceId: string,
  repoId: string,
  newPath: string,
): Promise<void> {
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

export function testSshConnection(
  host: string,
  user: string,
): Promise<SshTestResult> {
  return invoke<SshTestResult>("cmd_test_ssh_connection", { host, user });
}