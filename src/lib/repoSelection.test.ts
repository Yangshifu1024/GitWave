import { describe, expect, it } from "vitest";

import type { RepoRef } from "./api";
import { pickRestoredRepo, upsertRepos } from "./repoSelection";

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

describe("upsertRepos", () => {
  it("appends a new repo at the end", () => {
    const repos = [repo("a", "active"), repo("b", "active")];
    expect(upsertRepos(repos, [repo("c", "active")]).map((r) => r.id)).toEqual(["a", "b", "c"]);
  });

  it("appends every repo of a batch add", () => {
    const repos = [repo("a", "active")];
    expect(upsertRepos(repos, [repo("b", "active"), repo("c", "active")]).map((r) => r.id)).toEqual(
      ["a", "b", "c"],
    );
  });

  it("replaces an existing repo in place so its tab does not move", () => {
    const repos = [repo("a", "active"), repo("b", "missing"), repo("c", "active")];
    const revived = { ...repo("b", "active"), nickname: "relinked" };
    const merged = upsertRepos(repos, [revived]);
    expect(merged.map((r) => r.id)).toEqual(["a", "b", "c"]);
    expect(merged.find((r) => r.id === "b")).toEqual(revived);
  });

  it("returns the same array when nothing was added", () => {
    const repos = [repo("a", "active")];
    expect(upsertRepos(repos, [])).toBe(repos);
  });

  it("keeps the selection resolvable right after an add", () => {
    // The regression this guards: the tab strip's liveness guard falls back to
    // the first repo whenever the active id is absent from the rendered list.
    const before = [repo("a", "active"), repo("b", "active")];
    const added = repo("c", "active");
    expect(pickRestoredRepo(upsertRepos(before, [added]), added.id)).toBe(added.id);
    expect(pickRestoredRepo(upsertRepos(before, [added]), "b")).toBe("b");
  });
});
