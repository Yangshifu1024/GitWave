import { describe, expect, it } from "vitest";
import { useUiStore } from "./uiStore";

describe("uiStore menuAction channel", () => {
  it("assigns a fresh id on every request so repeating an action re-triggers", () => {
    useUiStore.getState().requestMenuAction("repo:fetch");
    const first = useUiStore.getState().menuAction;
    useUiStore.getState().requestMenuAction("repo:fetch");
    const second = useUiStore.getState().menuAction;

    expect(first).not.toBeNull();
    expect(second!.id).toBeGreaterThan(first!.id);
    expect(second!.action).toBe("repo:fetch");

    useUiStore.setState({ menuAction: null });
  });

  it("clearMenuAction only clears the matching request, not a newer one", () => {
    useUiStore.getState().requestMenuAction("branch:pull");
    const stale = useUiStore.getState().menuAction!;
    useUiStore.getState().requestMenuAction("branch:push");

    useUiStore.getState().clearMenuAction(stale.id);
    expect(useUiStore.getState().menuAction?.action).toBe("branch:push");

    useUiStore.getState().clearMenuAction(useUiStore.getState().menuAction!.id);
    expect(useUiStore.getState().menuAction).toBeNull();
  });
});
