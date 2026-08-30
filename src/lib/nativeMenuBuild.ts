// Pure builders that turn the shared menu spec (appMenuSpec.ts) into Tauri
// native menu options. Imports from @tauri-apps/api/menu are type-only, so
// this module stays runtime-dependency-free and unit-testable in plain
// node. The macOS hook (useNativeAppMenu.ts) owns the actual Menu.new /
// setAsAppMenu lifecycle. Spec labels arrive already translated; the
// app-menu strings below translate here via i18next.

import i18next from "i18next";
import type {
  MenuItemOptions,
  MenuOptions,
  PredefinedMenuItemOptions,
  SubmenuOptions,
} from "@tauri-apps/api/menu";

import {
  dispatchAppMenuItem,
  type AppMenuItemHandler,
  type AppMenuItemSpec,
  type AppMenuSpec,
} from "@/lib/appMenuSpec";

type NativeItem = MenuItemOptions | PredefinedMenuItemOptions;

const separator = (): PredefinedMenuItemOptions => ({ item: "Separator" });

function predefined(item: PredefinedMenuItemOptions["item"]): PredefinedMenuItemOptions {
  return { item };
}

function nativeEntry(itemSpec: AppMenuItemSpec, handler: AppMenuItemHandler): MenuItemOptions {
  return {
    id: itemSpec.id,
    text: itemSpec.label,
    enabled: itemSpec.enabled,
    action: () => dispatchAppMenuItem(itemSpec.id, handler),
  };
}

function submenuOptions(menu: AppMenuSpec, handler: AppMenuItemHandler): SubmenuOptions {
  return {
    id: menu.id,
    text: menu.label,
    items: menu.entries.map((entry) =>
      "separator" in entry ? separator() : nativeEntry(entry, handler),
    ),
  };
}

// App menu (right of the Apple menu): the File menu's self-handled items
// move here per macOS HIG, plus the OS-provided window-management items.
// Predefined items get muda-provided titles ("Quit GitWave", ⌘Q, "Hide
// GitWave", …).
function appMenuEntries(handler: AppMenuItemHandler): NativeItem[] {
  const t = i18next.t.bind(i18next);
  return [
    {
      id: "about",
      text: t("menu.app.aboutGitWave"),
      action: () => dispatchAppMenuItem("about", handler),
    },
    {
      id: "check-updates",
      text: t("menu.app.checkUpdates"),
      action: () => dispatchAppMenuItem("check-updates", handler),
    },
    separator(),
    {
      id: "settings",
      text: t("menu.app.settings"),
      accelerator: "CmdOrCtrl+,",
      action: () => dispatchAppMenuItem("settings", handler),
    },
    separator(),
    predefined("Hide"),
    predefined("HideOthers"),
    predefined("ShowAll"),
    separator(),
    predefined("Quit"),
  ];
}

// Standard Edit menu, predefined items only. Replacing Tauri's default menu
// removes its Edit entries, and without them the copy/cut/paste/select-all/
// undo shortcuts no longer reach the webview's text controls (user-verified
// regression). Predefined items get muda-provided titles (localized for
// Hide/Quit/About, English elsewhere — same as Tauri's default menu) and
// act on the first responder; nothing app-specific to maintain.
function editMenuEntries(): PredefinedMenuItemOptions[] {
  return [
    predefined("Undo"),
    predefined("Redo"),
    separator(),
    predefined("Cut"),
    predefined("Copy"),
    predefined("Paste"),
    predefined("SelectAll"),
  ];
}

// Standard Window menu so window shortcuts survive the default-menu
// replacement: Minimize (⌘M), Zoom, Enter Full Screen (⌃⌘F, lost with the
// default menu's View entry), then Bring All to Front as the HIG tail item.
// No Close Window item: the main window is the app (recorded in F007).
function windowMenuEntries(): PredefinedMenuItemOptions[] {
  return [
    predefined("Minimize"),
    predefined("Maximize"),
    predefined("Fullscreen"),
    separator(),
    predefined("BringAllToFront"),
  ];
}

/**
 * Assemble the full macOS application menu: GitWave app menu, standard Edit,
 * the spec's Workspace / Repository / Branch (the File spec menu is not
 * ported — its items live in the app menu), then a standard Window menu.
 */
export function buildNativeAppMenuOptions(
  spec: AppMenuSpec[],
  handler: AppMenuItemHandler,
): MenuOptions {
  const t = i18next.t.bind(i18next);
  return {
    id: "gitwave-app-menu",
    items: [
      { id: "app", text: "GitWave", items: appMenuEntries(handler) },
      { id: "edit", text: t("menu.menus.edit"), items: editMenuEntries() },
      // The File spec menu is skipped: its items live in the app menu.
      ...spec.filter((menu) => menu.id !== "file").map((menu) => submenuOptions(menu, handler)),
      { id: "window", text: t("menu.menus.window"), items: windowMenuEntries() },
    ],
  };
}
