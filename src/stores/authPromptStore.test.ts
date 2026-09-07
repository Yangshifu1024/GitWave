import { afterEach, describe, expect, it } from "vitest";

import { useAuthPromptStore } from "./authPromptStore";

afterEach(() => {
  useAuthPromptStore.getState().close();
});

describe("authPromptStore", () => {
  it("cancel() notifies onDismiss and clears the slot", () => {
    let dismissed = false;
    useAuthPromptStore.getState().show(
      "origin",
      () => {},
      () => (dismissed = true),
    );

    useAuthPromptStore.getState().cancel();

    expect(dismissed).toBe(true);
    expect(useAuthPromptStore.getState().remoteName).toBeNull();
    expect(useAuthPromptStore.getState().retry).toBeNull();
  });

  it("close() clears without notifying onDismiss (the submit path)", () => {
    let dismissed = false;
    useAuthPromptStore.getState().show(
      "origin",
      () => {},
      () => (dismissed = true),
    );

    useAuthPromptStore.getState().close();

    expect(dismissed).toBe(false);
    expect(useAuthPromptStore.getState().retry).toBeNull();
  });

  it("show() settles a previous pending registrant as cancelled", () => {
    let firstDismissed = false;
    useAuthPromptStore.getState().show(
      "origin",
      () => {},
      () => (firstDismissed = true),
    );

    // A second operation claiming the singleton dialog must not silently
    // drop the first one's callbacks — its awaiting caller would hang.
    useAuthPromptStore.getState().show("github.com", () => {});

    expect(firstDismissed).toBe(true);
    expect(useAuthPromptStore.getState().remoteName).toBe("github.com");
  });

  it("show() without onDismiss still clears a previous registrant", () => {
    let firstDismissed = false;
    useAuthPromptStore.getState().show(
      "origin",
      () => {},
      () => (firstDismissed = true),
    );

    useAuthPromptStore.getState().show("github.com", () => {});

    expect(firstDismissed).toBe(true);
    expect(useAuthPromptStore.getState().onDismiss).toBeNull();
  });
});
