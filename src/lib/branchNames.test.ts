import { describe, expect, it } from "vitest";
import {
  filterRemoteBranches,
  localNameForRemote,
  remoteShortName,
  splitBranchPrefix,
} from "./branchNames";

describe("remoteShortName", () => {
  it("strips the remote prefix from a remote-tracking name", () => {
    expect(remoteShortName("origin/main")).toBe("main");
  });

  it("keeps nested segments after the remote name", () => {
    expect(remoteShortName("origin/feat/x")).toBe("feat/x");
  });

  it("returns names without a slash unchanged", () => {
    expect(remoteShortName("main")).toBe("main");
  });

  it("supports remotes other than origin", () => {
    expect(remoteShortName("upstream/main")).toBe("main");
  });

  it("degrades to an empty short name for degenerate inputs", () => {
    expect(remoteShortName("")).toBe("");
    expect(remoteShortName("origin/")).toBe("");
  });
});

describe("localNameForRemote", () => {
  it("strips the configured remote prefix", () => {
    expect(localNameForRemote("origin/feat/x", ["origin"])).toBe("feat/x");
  });

  it("uses the longest configured prefix for nested remote names", () => {
    expect(localNameForRemote("foo/bar/x", ["foo", "foo/bar"])).toBe("x");
    expect(localNameForRemote("foo/bar/x", ["foo/bar", "foo"])).toBe("x");
  });

  it("prefers the configured remote over a same-named branch prefix", () => {
    // Remote `foo` + branch `foo/bar` on it: `foo/bar` is the branch name.
    expect(localNameForRemote("foo/bar", ["foo"])).toBe("bar");
  });

  it("falls back to the first-segment split without a matching remote", () => {
    expect(localNameForRemote("origin/main", [])).toBe("main");
    expect(localNameForRemote("orphan/feature", ["origin"])).toBe("feature");
  });
});

describe("filterRemoteBranches", () => {
  const local = (name: string) => ({ name, kind: "local" as const });
  const remote = (name: string) => ({ name, kind: "remote" as const });

  it("hides a remote branch that shares its short name with a local branch", () => {
    expect(filterRemoteBranches([local("main"), remote("origin/main")])).toEqual([local("main")]);
  });

  it("hides nested remote names matching a local branch", () => {
    expect(filterRemoteBranches([local("feat/x"), remote("origin/feat/x")])).toEqual([
      local("feat/x"),
    ]);
  });

  it("keeps remote branches without a local counterpart", () => {
    expect(filterRemoteBranches([local("main"), remote("origin/next")])).toEqual([
      local("main"),
      remote("origin/next"),
    ]);
  });

  it("hides the same short name across every remote", () => {
    expect(
      filterRemoteBranches([local("main"), remote("origin/main"), remote("upstream/main")]),
    ).toEqual([local("main")]);
  });

  it("keeps everything in a remote-only repo", () => {
    expect(filterRemoteBranches([remote("origin/main"), remote("origin/next")])).toEqual([
      remote("origin/main"),
      remote("origin/next"),
    ]);
  });
});

describe("splitBranchPrefix", () => {
  it("splits the first path segment as the prefix", () => {
    expect(splitBranchPrefix("feat/login-flow")).toEqual({
      prefix: "feat",
      rest: "login-flow",
    });
  });

  it("keeps deeper segments in the remainder", () => {
    expect(splitBranchPrefix("feat/auth/login")).toEqual({
      prefix: "feat",
      rest: "auth/login",
    });
  });

  it("returns no prefix for names without a slash", () => {
    expect(splitBranchPrefix("main")).toEqual({ prefix: null, rest: "main" });
  });
});
