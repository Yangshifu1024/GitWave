// Typed wrappers around Tauri commands. Mirrors the Rust types in
// src-tauri/src/domain/workspace.rs and src-tauri/src/domain/error.rs.
//
// Errors thrown by invoke follow `AppError`'s serialized shape:
//   { category: string, message: string, trace_id: string }

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
  path: string;
  nickname: string | null;
  settings_override: WorkspaceSettings | null;
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