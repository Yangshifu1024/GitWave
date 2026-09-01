import { describe, expect, it } from "vitest";

import {
  copyCommitInfoText,
  copyToClipboard,
  gateCommitCheckout,
  parseRemoteBranchName,
} from "@/lib/commitMenu";

describe("parseRemoteBranchName", () => {
  it("splits a remote-tracking shorthand", () => {
    expect(parseRemoteBranchName("origin/main")).toEqual({ remote: "origin", branch: "main" });
  });

  it("keeps slashes inside the branch name", () => {
    expect(parseRemoteBranchName("origin/feature/x")).toEqual({
      remote: "origin",
      branch: "feature/x",
    });
  });

  it("rejects names without a remote part", () => {
    expect(parseRemoteBranchName("main")).toBeNull();
  });

  it("rejects empty remote or branch segments", () => {
    expect(parseRemoteBranchName("/main")).toBeNull();
    expect(parseRemoteBranchName("origin/")).toBeNull();
  });
});

describe("copyCommitInfoText", () => {
  const commit = {
    sha: "abc1234567890",
    author: "Jane",
    time: 1700000000,
    message_summary: "fix: something",
  };

  it("renders sha, author, date and summary lines", () => {
    const text = copyCommitInfoText(commit);
    expect(text).toContain("abc1234567890");
    expect(text).toContain("Author: Jane");
    expect(text).toContain(`Date: ${new Date(commit.time * 1000).toLocaleString()}`);
    expect(text).toContain("fix: something");
    expect(text).toBe(
      `abc1234567890\nAuthor: Jane\nDate: ${new Date(commit.time * 1000).toLocaleString()}\n\nfix: something`,
    );
  });
});

describe("copyToClipboard", () => {
  it("reports false when the clipboard API is missing", async () => {
    const original = navigator.clipboard;
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      get: () => undefined,
    });
    try {
      await expect(copyToClipboard("x")).resolves.toBe(false);
    } finally {
      Object.defineProperty(navigator, "clipboard", {
        configurable: true,
        get: () => original,
      });
    }
  });
});

describe("gateCommitCheckout", () => {
  const clean = {
    isHead: false,
    dirtyCount: 0,
    mergeInProgress: false,
    rebasePaused: false,
  };

  it("is a noop when the commit is the current HEAD", () => {
    expect(gateCommitCheckout({ ...clean, isHead: true })).toEqual({ kind: "noop" });
  });

  it("blocks while a merge is in progress", () => {
    expect(gateCommitCheckout({ ...clean, mergeInProgress: true })).toEqual({
      kind: "blocked",
      reason: "merge",
    });
  });

  it("blocks while an interactive rebase is paused", () => {
    expect(gateCommitCheckout({ ...clean, rebasePaused: true })).toEqual({
      kind: "blocked",
      reason: "rebase",
    });
  });

  it("routes a dirty worktree to the three-choice dialog", () => {
    expect(gateCommitCheckout({ ...clean, dirtyCount: 3 })).toEqual({
      kind: "dirty",
      fileCount: 3,
    });
  });

  it("proceeds on a clean worktree", () => {
    expect(gateCommitCheckout(clean)).toEqual({ kind: "proceed" });
  });
});
