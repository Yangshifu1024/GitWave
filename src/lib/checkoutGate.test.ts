import { describe, expect, it } from "vitest";
import { gateCheckout, type CheckoutGateInput } from "./checkoutGate";

const base: CheckoutGateInput = {
  isCurrent: false,
  branchKind: "local",
  dirtyCount: 0,
  mergeInProgress: false,
  rebasePaused: false,
  occupiedWorktree: null,
};

describe("gateCheckout", () => {
  it("no-ops when the branch is already current", () => {
    expect(gateCheckout({ ...base, isCurrent: true })).toEqual({ kind: "noop" });
  });

  it("blocks remote-tracking branches", () => {
    const gate = gateCheckout({ ...base, branchKind: "remote" });
    expect(gate.kind).toBe("blocked");
    if (gate.kind === "blocked") expect(gate.reason).toBe("remote");
  });

  it("blocks merge in progress before dirty work", () => {
    const gate = gateCheckout({ ...base, mergeInProgress: true, dirtyCount: 3 });
    expect(gate.kind).toBe("blocked");
    if (gate.kind === "blocked") expect(gate.reason).toBe("merge");
  });

  it("blocks a paused interactive rebase", () => {
    const gate = gateCheckout({ ...base, rebasePaused: true });
    expect(gate.kind).toBe("blocked");
    if (gate.kind === "blocked") expect(gate.reason).toBe("rebase");
  });

  it("blocks a branch checked out in another worktree", () => {
    const gate = gateCheckout({ ...base, occupiedWorktree: "hotfix" });
    expect(gate.kind).toBe("blocked");
    if (gate.kind === "blocked") expect(gate.reason).toBe("worktree");
  });

  it("asks what to do with uncommitted files", () => {
    expect(gateCheckout({ ...base, dirtyCount: 2 })).toEqual({ kind: "dirty", fileCount: 2 });
  });

  it("proceeds when the working tree is clean", () => {
    expect(gateCheckout(base)).toEqual({ kind: "proceed" });
  });
});
