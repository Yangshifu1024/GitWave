// @vitest-environment jsdom
import { cleanup, render, waitFor } from "@testing-library/react";
import { StrictMode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { useUiStore } from "@/stores/uiStore";
import { installNativeAppMenuEarly, useNativeAppMenu } from "./useNativeAppMenu";

const native = vi.hoisted(() => {
  type Options = { id?: string; text?: string; action?: (id: string) => void; items?: Options[] };
  const channels = new Map<string, NonNullable<Options["action"]>>();
  let serial = 0;
  class Resource {
    rid = ++serial;
    closed = false;
    constructor(readonly options: Options) {
      if (options.id && options.action) channels.set(options.id, options.action);
    }
    close() {
      this.closed = true;
      if (this.options.id) channels.delete(this.options.id);
      return Promise.resolve();
    }
  }
  const resources: Resource[] = [];
  const menus: Resource[] = [];
  let current: Resource | null = null;
  function dropInline(items: Options[]) {
    for (const item of items) {
      if (item instanceof Resource) continue;
      if (item.id && item.action) {
        channels.set(item.id, item.action);
        // Tauri drops the temporary Rust wrapper after consuming inline options.
        channels.delete(item.id);
      }
      dropInline(item.items ?? []);
    }
  }
  class Menu extends Resource {
    static new(options: Options) {
      dropInline(options.items ?? []);
      const menu = new Menu(options);
      menus.push(menu);
      return Promise.resolve(menu);
    }
    setAsAppMenu() {
      const previous = current;
      // The native application retains the installed menu resource.
      // eslint-disable-next-line @typescript-eslint/no-this-alias
      current = this;
      return Promise.resolve(previous);
    }
  }
  class Item extends Resource {
    static new(options: Options) {
      dropInline(options.items ?? []);
      const item = new Item(options);
      resources.push(item);
      return Promise.resolve(item);
    }
  }
  return {
    Menu,
    Item,
    channels,
    resources,
    menus,
    gating: { noWorkspace: false, noRepo: false, detached: false, hasSha: true, syncBusy: false },
    current: () => current,
  };
});

vi.mock("@tauri-apps/api/menu", () => ({
  Menu: native.Menu,
  MenuItem: native.Item,
  PredefinedMenuItem: native.Item,
  Submenu: native.Item,
}));
vi.mock("@/lib/platform", () => ({ isMacOS: () => true }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ i18n: { language: "en" } }) }));
vi.mock("./useAppMenuGating", () => ({ useAppMenuGating: () => native.gating }));
vi.mock("@/hooks/useUpdater", () => ({ runUpdateCheck: vi.fn() }));
vi.mock("@/stores/workspaceStore", () => ({
  readLastActive: () => ({ workspaceId: "ws", repoId: "repo" }),
}));
vi.mock("@/lib/appMenuSpec", () => ({ buildAppMenuSpec: () => [] }));
vi.mock("@/lib/nativeMenuBuild", () => {
  let generation = 0;
  return {
    buildNativeAppMenuOptions: (_spec: unknown, handler: { openSettings: () => void }) => {
      const prefix = `menu-${++generation}`;
      return {
        id: prefix,
        items: [
          {
            id: `${prefix}-app`,
            text: "GitWave",
            items: [
              { id: `${prefix}-settings`, text: "Settings", action: handler.openSettings },
              { item: "Quit" },
            ],
          },
        ],
      };
    },
  };
});

afterEach(() => {
  cleanup();
  useUiStore.getState().setSettingsOpen(false);
});

it("retains native action channels through startup, StrictMode and gating replacement", async () => {
  const clickSettings = () => {
    useUiStore.getState().setSettingsOpen(false);
    const id = `${native.current()!.options.id}-settings`;
    expect(native.channels.has(id)).toBe(true);
    native.channels.get(id)!(id);
    expect(useUiStore.getState().settingsOpen).toBe(true);
  };
  installNativeAppMenuEarly();
  await waitFor(() => expect(native.menus).toHaveLength(1));
  clickSettings();
  const startupChildren = [...native.resources];

  function Harness() {
    useNativeAppMenu({ onAbout: () => undefined });
    return null;
  }
  const view = render(
    <StrictMode>
      <Harness />
    </StrictMode>,
  );
  await waitFor(() => expect(native.menus).toHaveLength(3));
  await waitFor(() => expect(startupChildren.every((item) => item.closed)).toBe(true));
  clickSettings();
  const previousChildren = native.resources.filter((item) => !item.closed);
  const previousMenu = native.current()!;

  native.gating = { ...native.gating, noRepo: true };
  view.rerender(
    <StrictMode>
      <Harness />
    </StrictMode>,
  );
  await waitFor(() => expect(native.menus).toHaveLength(4));
  await waitFor(() => expect(previousChildren.every((item) => item.closed)).toBe(true));
  expect(previousMenu.closed).toBe(true);
  clickSettings();
  expect(native.resources.filter((item) => !item.closed)).toHaveLength(3);
  expect(native.channels).toHaveLength(1);
});
