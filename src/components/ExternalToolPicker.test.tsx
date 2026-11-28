// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { listEditors, listTerminals, openInEditor, openInTerminal } from "@/lib/api";
import { ExternalToolPicker } from "./ExternalToolPicker";

vi.mock("@/lib/api", () => ({
  listEditors: vi.fn(),
  listTerminals: vi.fn(),
  openInEditor: vi.fn(),
  openInTerminal: vi.fn(),
  formatAppError: (error: unknown) => (error instanceof Error ? error.message : String(error)),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

const terminals = [
  { id: "kitty", name: "Kitty" },
  { id: "wezterm", name: "WezTerm" },
];
const editors = [
  { id: "code", name: "VS Code" },
  { id: "zed", name: "Zed" },
];
const onError = vi.fn<(message: string) => void>();
type Kind = "editor" | "terminal";
function mount(kinds: Kind[] = ["terminal"], repoPath: string | null = "/repo") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      {kinds.map((kind) => (
        <ExternalToolPicker key={kind} kind={kind} repoPath={repoPath} onError={onError} />
      ))}
    </QueryClientProvider>,
  );
}
function picker(kind: Kind) {
  const prefix = kind === "terminal" ? "workspace.openTerminal" : "workspace.openEditor";
  return within(screen.getByRole("group", { name: `${prefix}.label` }));
}
const buttons = (kind: Kind) => picker(kind).getAllByRole<HTMLButtonElement>("button");
function menuTrigger(kind: Kind) {
  const scope = picker(kind);
  return (
    scope.queryByRole("button", { name: /\.choose$/ }) ??
    scope.getByRole("button", { name: /\.label$/ })
  );
}
async function choose(kind: Kind, name: string) {
  fireEvent.click(menuTrigger(kind));
  const choice = await screen.findByRole("menuitem", { name: new RegExp(name) });
  fireEvent.click(choice);
}

beforeEach(() => {
  vi.resetAllMocks();
  localStorage.clear();
  vi.mocked(listTerminals).mockResolvedValue(terminals);
  vi.mocked(listEditors).mockResolvedValue(editors);
  vi.mocked(openInTerminal).mockResolvedValue(undefined);
  vi.mocked(openInEditor).mockResolvedValue(undefined);
});
afterEach(cleanup);

it("selects explicitly on first use and restores independent terminal and editor choices after remount", async () => {
  const view = mount(["terminal", "editor"]);
  for (const kind of ["terminal", "editor"] as const) expect(buttons(kind)).toHaveLength(1);
  expect(openInTerminal).not.toHaveBeenCalled();
  expect(openInEditor).not.toHaveBeenCalled();
  await choose("terminal", "WezTerm");
  await waitFor(() => expect(buttons("terminal")).toHaveLength(2));
  expect(openInTerminal).toHaveBeenCalledExactlyOnceWith("/repo", "wezterm");
  expect(buttons("editor")).toHaveLength(1);
  await choose("editor", "Zed");
  await waitFor(() => expect(buttons("editor")).toHaveLength(2));
  expect(openInEditor).toHaveBeenCalledExactlyOnceWith("/repo", "zed");
  view.unmount();
  mount(["terminal", "editor"], "/new-repo");
  await waitFor(() => expect(buttons("terminal")).toHaveLength(2));
  await waitFor(() => expect(buttons("editor")).toHaveLength(2));
  fireEvent.click(picker("terminal").getByRole("button", { name: "WezTerm" }));
  fireEvent.click(picker("editor").getByRole("button", { name: "Zed" }));
  await waitFor(() => expect(openInTerminal).toHaveBeenLastCalledWith("/new-repo", "wezterm"));
  expect(openInEditor).toHaveBeenLastCalledWith("/new-repo", "zed");
  await choose("terminal", "Kitty");
  await waitFor(() => expect(openInTerminal).toHaveBeenLastCalledWith("/new-repo", "kitty"));
  fireEvent.click(picker("terminal").getByRole("button", { name: "Kitty" }));
  await waitFor(() =>
    expect(vi.mocked(openInTerminal).mock.calls.slice(-2)).toEqual([
      ["/new-repo", "kitty"],
      ["/new-repo", "kitty"],
    ]),
  );
});

it("keeps successful preferences when an alternative fails and first-use failures do not set a preference", async () => {
  mount(["terminal", "editor"]);
  vi.mocked(openInEditor).mockRejectedValueOnce(new Error("launch failed"));
  await choose("editor", "Zed");
  await waitFor(() => expect(onError).toHaveBeenCalledExactlyOnceWith("launch failed"));
  expect(buttons("editor")).toHaveLength(1);
  await choose("terminal", "Kitty");
  await waitFor(() => expect(buttons("terminal")).toHaveLength(2));
  vi.mocked(openInTerminal).mockRejectedValueOnce(new Error("new terminal failed"));
  await choose("terminal", "WezTerm");
  await waitFor(() => expect(onError).toHaveBeenLastCalledWith("new terminal failed"));
  fireEvent.click(picker("terminal").getByRole("button", { name: "Kitty" }));
  await waitFor(() => expect(openInTerminal).toHaveBeenLastCalledWith("/repo", "kitty"));
});

it("falls back to the menu when the previously selected application is no longer detected", async () => {
  const view = mount();
  await choose("terminal", "WezTerm");
  await waitFor(() => expect(buttons("terminal")).toHaveLength(2));
  view.unmount();
  vi.mocked(listTerminals).mockResolvedValue([terminals[0]!]);
  mount();
  fireEvent.click(menuTrigger("terminal"));
  const choice = await screen.findByRole("menuitem", { name: "Kitty" });
  expect(screen.queryByRole("menuitem", { name: /WezTerm/ })).toBeNull();
  expect(screen.queryByRole("button", { name: "WezTerm", hidden: true })).toBeNull();
  fireEvent.click(choice);
  expect(openInTerminal).toHaveBeenCalledTimes(2);
  expect(openInTerminal).toHaveBeenLastCalledWith("/repo", "kitty");
});

it("disables both kinds without an active repository", () => {
  mount(["terminal", "editor"], null);
  for (const kind of ["terminal", "editor"] as const) {
    for (const button of buttons(kind)) {
      expect(button.disabled).toBe(true);
      fireEvent.click(button);
    }
  }
  expect(screen.queryByRole("menu")).toBeNull();
  expect(openInTerminal).not.toHaveBeenCalled();
  expect(openInEditor).not.toHaveBeenCalled();
});

it("disables launch and switching while the previous launch is pending", async () => {
  mount();
  await choose("terminal", "Kitty");
  const launch = await picker("terminal").findByRole<HTMLButtonElement>("button", {
    name: "Kitty",
  });
  let finishLaunch!: () => void;
  vi.mocked(openInTerminal).mockImplementationOnce(
    () =>
      new Promise<void>((resolve) => {
        finishLaunch = resolve;
      }),
  );
  fireEvent.click(launch);
  expect(launch.disabled).toBe(true);
  expect((menuTrigger("terminal") as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(launch);
  fireEvent.click(menuTrigger("terminal"));
  expect(openInTerminal).toHaveBeenCalledTimes(2);
  expect(screen.queryByRole("menu")).toBeNull();
  finishLaunch();
  await waitFor(() => expect(launch.disabled).toBe(false));
});

it("displays the empty discovery state without launching an application", async () => {
  vi.mocked(listEditors).mockResolvedValue([]);
  mount(["editor"]);
  fireEvent.click(menuTrigger("editor"));
  const empty = await screen.findByRole("menuitem", { name: "workspace.openEditor.noneDetected" });
  expect(empty.getAttribute("aria-disabled")).toBe("true");
  fireEvent.click(empty);
  expect(openInEditor).not.toHaveBeenCalled();
});
