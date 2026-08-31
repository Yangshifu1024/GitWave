import { describe, expect, it } from "vitest";

import type { RepoRef } from "./api";
import { pickRestoredRepo } from "./repoSelection";

function repo(id: string, status: RepoRef["status"]): RepoRef {
  return {
    id,
    workspace_id: "ws-1",
    path: `C:/repos/${id}`,
    nickname: null,
    settings_override: null,
    status,
    missing_since: status === "missing" ? 1 : null,
    added_at: 1,
  };
}

describe("pickRestoredRepo", () => {
  it("keeps the target when it is present and active", () => {
    const repos = [repo("a", "active"), repo("b", "active")];
    expect(pickRestoredRepo(repos, "b")).toBe("b");
  });

  it("falls back to the first active repo when the target is missing", () => {
    const repos = [repo("a", "missing"), repo("b", "active"), repo("c", "active")];
    expect(pickRestoredRepo(repos, "a")).toBe("b");
  });

  it("falls back when the target does not exist (stale persisted id)", () => {
    const repos = [repo("a", "active"), repo("b", "missing")];
    expect(pickRestoredRepo(repos, "gone")).toBe("a");
  });

  it("falls back when the target is null", () => {
    const repos = [repo("a", "missing"), repo("b", "active")];
    expect(pickRestoredRepo(repos, null)).toBe("b");
  });

  it("returns null when every repo is missing", () => {
    const repos = [repo("a", "missing"), repo("b", "missing")];
    expect(pickRestoredRepo(repos, "a")).toBeNull();
    expect(pickRestoredRepo(repos, null)).toBeNull();
  });

  it("returns null for an empty repo list", () => {
    expect(pickRestoredRepo([], null)).toBeNull();
    expect(pickRestoredRepo([], "a")).toBeNull();
  });
});
