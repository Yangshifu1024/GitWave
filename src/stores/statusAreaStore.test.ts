import { beforeEach, describe, expect, it } from "vitest";
import { useStatusAreaStore } from "./statusAreaStore";

describe("statusAreaStore", () => {
  beforeEach(() => {
    useStatusAreaStore.getState().clearStatus();
  });

  it("defaults to the success variant", () => {
    useStatusAreaStore.getState().setStatus("Fetched from origin");
    expect(useStatusAreaStore.getState().status).toEqual({
      text: "Fetched from origin",
      variant: "success",
    });
  });

  it("carries danger and info variants through", () => {
    useStatusAreaStore.getState().setStatus("Push failed", "danger");
    expect(useStatusAreaStore.getState().status?.variant).toBe("danger");

    useStatusAreaStore.getState().setStatus("Primary AI provider failed", "info");
    expect(useStatusAreaStore.getState().status?.variant).toBe("info");
  });

  it("persists until overwritten and clearStatus resets it", () => {
    useStatusAreaStore.getState().setStatus("first");
    useStatusAreaStore.getState().setStatus("second", "danger");
    expect(useStatusAreaStore.getState().status?.text).toBe("second");

    useStatusAreaStore.getState().clearStatus();
    expect(useStatusAreaStore.getState().status).toBeNull();
  });
});
