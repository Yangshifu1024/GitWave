import { beforeEach, describe, expect, it } from "vitest";
import {
  EMPTY_SESSION,
  reconcileChangeSelection,
  useWorkbenchStore,
  workbenchKey,
} from "./workbenchStore";

describe("workbench sessions", () => {
  beforeEach(() => useWorkbenchStore.setState({ sessions: {} }));
  it("restores independent modes and selections across workspace/repo round trips", () => {
    const a = workbenchKey("one", "repo");
    const b = workbenchKey("two", "repo");
    const update = useWorkbenchStore.getState().update;
    update(a, {
      commit: "abc",
      historyFile: { commit: "abc", path: "history.ts" },
      mode: "changes",
      change: { path: "draft.ts", staged: false },
    });
    update(b, { commit: "other" });
    expect(useWorkbenchStore.getState().sessions[a]).toMatchObject({
      mode: "changes",
      commit: "abc",
      change: { path: "draft.ts", staged: false },
    });
    expect(useWorkbenchStore.getState().sessions[b]).toEqual({ ...EMPTY_SESSION, commit: "other" });
    update(a, { mode: "history" });
    expect(useWorkbenchStore.getState().sessions[a]?.historyFile?.path).toBe("history.ts");
    expect(useWorkbenchStore.getState().sessions[a]?.change?.path).toBe("draft.ts");
  });
  it("follows staged files without losing the side of partially staged files", () => {
    const selection = { path: "same.ts", staged: false };
    expect(reconcileChangeSelection(selection, [{ ...selection, staged: true }])).toEqual({
      path: "same.ts",
      staged: true,
    });
    expect(
      reconcileChangeSelection(selection, [selection, { ...selection, staged: true }]),
    ).toEqual(selection);
    expect(reconcileChangeSelection(selection, [{ path: "other.ts", staged: false }])).toBeNull();
  });
});
