import { describe, expect, it } from "vitest";
import type {
  MenuItemOptions,
  PredefinedMenuItemOptions,
  SubmenuOptions,
} from "@tauri-apps/api/menu";

import { initI18n } from "@/i18n";

import { buildAppMenuSpec, type AppMenuItemHandler, type MenuGating } from "./appMenuSpec";
import { buildNativeAppMenuOptions } from "./nativeMenuBuild";

// Labels translate at spec-build time; these assertions pin the English set.
// Top-level await keeps the locale deterministic regardless of the host's
// system language.
const i18n = initI18n();
if (i18n.language !== "en") await i18n.changeLanguage("en");

const ACTIVE: MenuGating = {
  noWorkspace: false,
  noRepo: false,
  detached: false,
  hasSha: true,
  syncBusy: false,
};

const NOOP_HANDLER: AppMenuItemHandler = {
  requestMenuAction: () => undefined,
  openSettings: () => undefined,
  openAbout: () => undefined,
  checkUpdates: () => undefined,
  quit: () => undefined,
};

function isPredefined(item: unknown): item is PredefinedMenuItemOptions {
  return typeof item === "object" && item !== null && "item" in item;
}

function isSubmenu(item: unknown): item is SubmenuOptions {
  return typeof item === "object" && item !== null && "items" in item;
}

function isItem(item: unknown): item is MenuItemOptions {
  return typeof item === "object" && item !== null && "text" in item && !("items" in item);
}

function submenuAt(items: unknown[], index: number): SubmenuOptions {
  const entry = items[index];
  if (!isSubmenu(entry)) throw new Error(`items[${index}] is not a submenu`);
  return entry;
}

function itemsByLabel(submenu: SubmenuOptions): Record<string, MenuItemOptions> {
  return Object.fromEntries((submenu.items ?? []).filter(isItem).map((item) => [item.text, item]));
}

describe("buildNativeAppMenuOptions", () => {
  const options = buildNativeAppMenuOptions(buildAppMenuSpec(ACTIVE), NOOP_HANDLER);
  const top = options.items ?? [];
  const topSubmenus = top.filter(isSubmenu);
  const topTexts = topSubmenus.map((submenu) => submenu.text);

  it("orders menus app / Edit / Workspace / Repository / Branch / Window and drops File", () => {
    expect(topTexts).toEqual(["GitWave", "Edit", "Workspace", "Repository", "Branch", "Window"]);
  });

  it("builds the app menu with About, Check for Updates, Settings ⌘, and OS tail items", () => {
    const entries = submenuAt(top, 0).items ?? [];
    const texts = entries.filter(isItem).map((item) => item.text);
    expect(texts).toEqual(["About GitWave", "Check for Updates…", "Settings…"]);

    const settings = entries.filter(isItem)[2];
    expect(settings?.accelerator).toBe("CmdOrCtrl+,");
    expect(typeof settings?.action).toBe("function");

    // Everything else is predefined (system-localized) or a separator.
    const predefined = entries.filter(isPredefined).map((item) => item.item);
    expect(predefined).toEqual([
      "Separator",
      "Separator",
      "Hide",
      "HideOthers",
      "ShowAll",
      "Separator",
      "Quit",
    ]);
  });

  it("builds a predefined-only Edit menu for webview text shortcuts", () => {
    const edit = submenuAt(top, 1);
    const kinds = (edit.items ?? []).map((entry) =>
      isPredefined(entry) ? entry.item : "unexpected",
    );
    expect(kinds).toEqual(["Undo", "Redo", "Separator", "Cut", "Copy", "Paste", "SelectAll"]);
  });

  it("maps spec entries to native items with label text and gating-driven enabled", () => {
    const byText = itemsByLabel(submenuAt(top, 2));
    expect(byText.New?.enabled).toBe(true);
    expect(byText.Import?.enabled).toBe(true);
    expect(byText.Delete?.enabled).toBe(true);

    const gated = buildNativeAppMenuOptions(
      buildAppMenuSpec({ ...ACTIVE, noWorkspace: true, noRepo: true }),
      NOOP_HANDLER,
    );
    const gatedByText = itemsByLabel(submenuAt(gated.items ?? [], 2));
    expect(gatedByText.New?.enabled).toBe(true);
    expect(gatedByText.Rename?.enabled).toBe(false);
    expect(gatedByText.Delete?.enabled).toBe(false);
  });

  it("builds a predefined-only Window menu", () => {
    const window = submenuAt(top, topTexts.indexOf("Window"));
    const kinds = (window.items ?? []).map((entry) =>
      isPredefined(entry) ? entry.item : "unexpected",
    );
    expect(kinds).toEqual(["Minimize", "Maximize", "Fullscreen", "Separator", "BringAllToFront"]);
  });

  it("wires spec actions through the handler via dispatch", () => {
    const routed: string[] = [];
    const menu = buildNativeAppMenuOptions(buildAppMenuSpec(ACTIVE), {
      ...NOOP_HANDLER,
      requestMenuAction: (action) => void routed.push(action),
    });
    const workspace = submenuAt(menu.items ?? [], 2);
    workspace.items
      ?.filter(isItem)
      .find((item) => item.text === "Import")
      ?.action?.("import");
    expect(routed).toEqual(["workspace:import"]);
  });
});
