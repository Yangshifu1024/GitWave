// Single source of truth for the app menu structure, shared by the in-app
// menu bar (Windows / Linux, AppMenuBar.tsx) and the macOS native system
// menu (useNativeAppMenu.ts), so both surfaces gate and dispatch identically
// by construction. Entries carry stable ids; non-action ids (settings /
// about / quit) are handled by their owners, everything else goes through
// the ui-store action bus that ActionBar consumes.

import type { AppMenuAction } from "@/stores/uiStore";

/** Self-handled entries that do not route through the action bus. */
export type AppMenuSpecialId = "settings" | "about" | "quit";

export type AppMenuItemId = AppMenuAction | AppMenuSpecialId;

export function isAppMenuAction(id: AppMenuItemId): id is AppMenuAction {
  switch (id) {
    case "settings":
    case "about":
    case "quit":
      return false;
    default: {
      // Compile-time exhaustiveness: a new AppMenuSpecialId without a case
      // above makes this assignment fail to compile.
      const action: AppMenuAction = id;
      void action;
      return true;
    }
  }
}

/** Routing targets for a menu item click; both menu surfaces supply these. */
export interface AppMenuItemHandler {
  requestMenuAction: (action: AppMenuAction) => void;
  openSettings: () => void;
  openAbout: () => void;
  /** In-app bar quits the app; the native menu's Quit is OS-handled (no-op). */
  quit: () => void;
}

/** Single click router shared by the in-app bar and the native menu. */
export function dispatchAppMenuItem(id: AppMenuItemId, handler: AppMenuItemHandler): void {
  if (isAppMenuAction(id)) handler.requestMenuAction(id);
  else if (id === "settings") handler.openSettings();
  else if (id === "about") handler.openAbout();
  else handler.quit();
}

export interface AppMenuItemSpec {
  id: AppMenuItemId;
  /** Long description; HeroUI typeahead text on Windows / Linux. */
  textValue: string;
  /** Short label shown on the item in both menus. */
  label: string;
  enabled: boolean;
  /** Rendered destructive (red) in the in-app menu; native macOS menus
   * don't support per-item color, only the text carries over there. */
  destructive?: boolean;
}

export interface AppMenuSeparator {
  separator: true;
}

export type AppMenuEntry = AppMenuItemSpec | AppMenuSeparator;

export interface AppMenuSpec {
  id: "file" | "workspace" | "repository" | "branch";
  label: string;
  entries: AppMenuEntry[];
}

/** Enable/disable inputs; mirrors what the ActionBar buttons allow. */
export interface MenuGating {
  noWorkspace: boolean;
  noRepo: boolean;
  detached: boolean;
  hasSha: boolean;
  syncBusy: boolean;
}

function item(
  id: AppMenuItemId,
  textValue: string,
  label: string,
  enabled: boolean,
  destructive?: boolean,
): AppMenuItemSpec {
  return { id, textValue, label, enabled, destructive };
}

const sep: AppMenuSeparator = { separator: true };

export function buildAppMenuSpec(gating: MenuGating): AppMenuSpec[] {
  const { noWorkspace, noRepo, detached, hasSha, syncBusy } = gating;
  return [
    {
      id: "file",
      label: "File",
      entries: [
        item("settings", "Settings", "Settings", true),
        item("about", "About", "About", true),
        sep,
        item("quit", "Exit", "Exit", true),
      ],
    },
    {
      id: "workspace",
      label: "Workspace",
      entries: [
        item("workspace:new", "New workspace", "New", true),
        item("workspace:rename", "Rename workspace", "Rename", !noWorkspace),
        item("workspace:ai", "AI provider", "AI", !noWorkspace),
        item("workspace:export", "Export workspace", "Export", !noWorkspace),
        item("workspace:import", "Import workspace", "Import", true),
        item("workspace:delete", "Delete workspace", "Delete", !noWorkspace, true),
      ],
    },
    {
      id: "repository",
      label: "Repository",
      entries: [
        item("repo:init", "Initialize new repo", "Init", !noWorkspace),
        item("repo:clone", "Clone remote repo", "Clone", !noWorkspace),
        item("repo:add", "Add existing local repo", "Add", !noWorkspace),
        item("repo:fetch", "Fetch", "Fetch", !noRepo && !syncBusy),
        item("repo:lfs", "Git LFS — track large files", "LFS", !noRepo),
        item("repo:hooks", "Git hooks editor", "Hooks", !noRepo),
        item("repo:worktree-new", "Create worktree", "New worktree", !noRepo),
      ],
    },
    {
      id: "branch",
      label: "Branch",
      entries: [
        item("branch:new", "New branch", "New", !noRepo && !detached && hasSha),
        item("branch:pull", "Pull", "Pull", !noRepo && !syncBusy && !detached),
        item("branch:push", "Push", "Push", !noRepo && !syncBusy && !detached),
        item("branch:pr", "AI PR description for the current branch", "PR", !noRepo && !detached),
      ],
    },
  ];
}
