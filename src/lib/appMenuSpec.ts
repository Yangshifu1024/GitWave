// Single source of truth for the app menu structure, shared by the in-app
// menu bar (Windows / Linux, AppMenuBar.tsx) and the macOS native system
// menu (useNativeAppMenu.ts), so both surfaces gate and dispatch identically
// by construction. Entries carry stable ids; non-action ids (settings /
// about / quit) are handled by their owners, everything else goes through
// the ui-store action bus that ActionBar consumes.
//
// Labels are translated at spec-build time; both consumers rebuild on
// language change, so a switch re-renders both menu surfaces.

import i18next from "i18next";

import type { AppMenuAction } from "@/stores/uiStore";

/** Self-handled entries that do not route through the action bus. */
export type AppMenuSpecialId = "settings" | "about" | "quit" | "check-updates";

export type AppMenuItemId = AppMenuAction | AppMenuSpecialId;

export function isAppMenuAction(id: AppMenuItemId): id is AppMenuAction {
  switch (id) {
    case "settings":
    case "about":
    case "quit":
    case "check-updates":
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
  /** Runs a manual update check; Settings opens so the outcome is visible. */
  checkUpdates: () => void;
  /** In-app bar quits the app; the native menu's Quit is OS-handled (no-op). */
  quit: () => void;
}

/** Single click router shared by the in-app bar and the native menu. */
export function dispatchAppMenuItem(id: AppMenuItemId, handler: AppMenuItemHandler): void {
  if (isAppMenuAction(id)) handler.requestMenuAction(id);
  else if (id === "settings") handler.openSettings();
  else if (id === "about") handler.openAbout();
  else if (id === "check-updates") handler.checkUpdates();
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
  const t = i18next.t.bind(i18next);
  return [
    {
      id: "file",
      label: t("menu.menus.file"),
      entries: [
        item("settings", t("menu.file.settings.text"), t("menu.file.settings.label"), true),
        item("about", t("menu.file.about.text"), t("menu.file.about.label"), true),
        item(
          "check-updates",
          t("menu.file.checkUpdates.text"),
          t("menu.file.checkUpdates.label"),
          true,
        ),
        sep,
        item("quit", t("menu.file.quit.text"), t("menu.file.quit.label"), true),
      ],
    },
    {
      id: "workspace",
      label: t("menu.menus.workspace"),
      entries: [
        item("workspace:new", t("menu.workspace.new.text"), t("menu.workspace.new.label"), true),
        item(
          "workspace:rename",
          t("menu.workspace.rename.text"),
          t("menu.workspace.rename.label"),
          !noWorkspace,
        ),
        item(
          "workspace:ai",
          t("menu.workspace.ai.text"),
          t("menu.workspace.ai.label"),
          !noWorkspace,
        ),
        item(
          "workspace:export",
          t("menu.workspace.export.text"),
          t("menu.workspace.export.label"),
          !noWorkspace,
        ),
        item(
          "workspace:import",
          t("menu.workspace.import.text"),
          t("menu.workspace.import.label"),
          true,
        ),
        item(
          "workspace:delete",
          t("menu.workspace.delete.text"),
          t("menu.workspace.delete.label"),
          !noWorkspace,
          true,
        ),
      ],
    },
    {
      id: "repository",
      label: t("menu.menus.repository"),
      entries: [
        item(
          "repo:init",
          t("menu.repository.init.text"),
          t("menu.repository.init.label"),
          !noWorkspace,
        ),
        item(
          "repo:clone",
          t("menu.repository.clone.text"),
          t("menu.repository.clone.label"),
          !noWorkspace,
        ),
        item(
          "repo:add",
          t("menu.repository.add.text"),
          t("menu.repository.add.label"),
          !noWorkspace,
        ),
        item(
          "repo:fetch",
          t("menu.repository.fetch.text"),
          t("menu.repository.fetch.label"),
          !noRepo && !syncBusy,
        ),
        item("repo:lfs", t("menu.repository.lfs.text"), t("menu.repository.lfs.label"), !noRepo),
        item(
          "repo:hooks",
          t("menu.repository.hooks.text"),
          t("menu.repository.hooks.label"),
          !noRepo,
        ),
        item(
          "repo:worktree-new",
          t("menu.repository.worktreeNew.text"),
          t("menu.repository.worktreeNew.label"),
          !noRepo,
        ),
      ],
    },
    {
      id: "branch",
      label: t("menu.menus.branch"),
      entries: [
        item(
          "branch:new",
          t("menu.branch.new.text"),
          t("menu.branch.new.label"),
          !noRepo && !detached && hasSha,
        ),
        item(
          "branch:pull",
          t("menu.branch.pull.text"),
          t("menu.branch.pull.label"),
          !noRepo && !syncBusy && !detached,
        ),
        item(
          "branch:push",
          t("menu.branch.push.text"),
          t("menu.branch.push.label"),
          !noRepo && !syncBusy && !detached,
        ),
        item(
          "branch:pr",
          t("menu.branch.pr.text"),
          t("menu.branch.pr.label"),
          !noRepo && !detached,
        ),
      ],
    },
  ];
}
