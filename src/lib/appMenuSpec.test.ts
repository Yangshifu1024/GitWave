import { describe, expect, it } from "vitest";

import {
  buildAppMenuSpec,
  dispatchAppMenuItem,
  type AppMenuItemSpec,
  type AppMenuSpec,
  type MenuGating,
} from "./appMenuSpec";

const ACTIVE: MenuGating = {
  noWorkspace: false,
  noRepo: false,
  detached: false,
  hasSha: true,
  syncBusy: false,
};

type MenuMap = Record<AppMenuSpec["id"], AppMenuSpec>;

function menuById(gating: MenuGating): MenuMap {
  return Object.fromEntries(buildAppMenuSpec(gating).map((menu) => [menu.id, menu])) as MenuMap;
}

function items(menu: AppMenuSpec): Array<Pick<AppMenuItemSpec, "id" | "enabled" | "destructive">> {
  return menu.entries.filter((e): e is AppMenuItemSpec => !("separator" in e));
}

describe("buildAppMenuSpec", () => {
  it("enables every item when an active repo sits on a named branch", () => {
    const menus = menuById(ACTIVE);
    for (const menu of Object.values(menus)) {
      for (const item of items(menu)) {
        expect(item.enabled, item.id).toBe(true);
      }
    }
  });

  it("keeps only New and Import in Workspace when no workspace is active", () => {
    // Realistic state: with no workspace there is no active repo either.
    const menus = menuById({ ...ACTIVE, noWorkspace: true, noRepo: true });
    const workspace = Object.fromEntries(
      items(menus.workspace).map((item) => [item.id, item.enabled]),
    );
    expect(workspace).toEqual({
      "workspace:new": true,
      "workspace:rename": false,
      "workspace:ai": false,
      "workspace:export": false,
      "workspace:import": true,
      "workspace:delete": false,
    });
    // Every Repository and Branch item requires a workspace (and repo).
    for (const menu of [menus.repository, menus.branch]) {
      for (const item of items(menu)) {
        expect(item.enabled, item.id).toBe(false);
      }
    }
  });

  it("gates repo-scoped items but keeps workspace-level ones without a repo", () => {
    const menus = menuById({ ...ACTIVE, noRepo: true });
    const repository = items(menus.repository).map((item) => [item.id, item.enabled]);
    expect(repository).toEqual([
      ["repo:init", true],
      ["repo:clone", true],
      ["repo:add", true],
      ["repo:fetch", false],
      ["repo:lfs", false],
      ["repo:hooks", false],
      ["repo:worktree-new", false],
    ]);
    for (const item of items(menus.branch)) {
      expect(item.enabled, item.id).toBe(false);
    }
  });

  it("disables sync items while a sync is in flight", () => {
    const menus = menuById({ ...ACTIVE, syncBusy: true });
    expect(items(menus.repository).find((i) => i.id === "repo:fetch")?.enabled).toBe(false);
    const branch = Object.fromEntries(items(menus.branch).map((i) => [i.id, i.enabled]));
    expect(branch).toEqual({
      "branch:new": true,
      "branch:pull": false,
      "branch:push": false,
      "branch:pr": true,
    });
  });

  it("disables branch ops when detached, and New branch additionally without a sha", () => {
    const detached = menuById({ ...ACTIVE, detached: true });
    for (const item of items(detached.branch)) {
      expect(item.enabled, item.id).toBe(false);
    }

    const noSha = menuById({ ...ACTIVE, hasSha: false });
    const branch = Object.fromEntries(items(noSha.branch).map((i) => [i.id, i.enabled]));
    expect(branch).toEqual({
      "branch:new": false,
      "branch:pull": true,
      "branch:push": true,
      "branch:pr": true,
    });
  });

  it("marks only Delete workspace destructive and keeps File items always enabled", () => {
    const menus = menuById(ACTIVE);
    const destructive = Object.values(menus).flatMap((menu) =>
      items(menu)
        .filter((item) => item.destructive)
        .map((item) => item.id),
    );
    expect(destructive).toEqual(["workspace:delete"]);

    for (const item of items(menus.file)) {
      expect(item.enabled, item.id).toBe(true);
    }
    expect(items(menus.file).map((item) => item.id)).toEqual(["settings", "about", "quit"]);
  });
});

describe("dispatchAppMenuItem", () => {
  function recordingHandler() {
    const routed: string[] = [];
    return {
      routed,
      handler: {
        requestMenuAction: (action: string): void => void routed.push(action),
        openSettings: (): void => void routed.push("settings"),
        openAbout: (): void => void routed.push("about"),
        quit: (): void => void routed.push("quit"),
      },
    };
  }

  it("routes action ids to the request bus and special ids to their owners", () => {
    const { routed, handler } = recordingHandler();
    dispatchAppMenuItem("repo:clone", handler);
    dispatchAppMenuItem("settings", handler);
    dispatchAppMenuItem("about", handler);
    dispatchAppMenuItem("quit", handler);
    expect(routed).toEqual(["repo:clone", "settings", "about", "quit"]);
  });

  it("routes every spec id somewhere without throwing", () => {
    const { routed, handler } = recordingHandler();
    for (const menu of buildAppMenuSpec(ACTIVE)) {
      for (const item of items(menu)) {
        dispatchAppMenuItem(item.id, handler);
      }
    }
    // Every entry produced exactly one route (17 actions + 3 File specials).
    expect(routed).toHaveLength(20);
  });
});
