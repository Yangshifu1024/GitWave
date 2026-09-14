import { describe, expect, it } from "vitest";

import { stashesQueryKey } from "./useStashes";

describe("stashesQueryKey", () => {
  it("includes the active repo so a repo switch refetches", () => {
    const a = stashesQueryKey("ws-1", "repo-a");
    const b = stashesQueryKey("ws-1", "repo-b");
    expect(a).not.toEqual(b);
    expect(a).toEqual(["stashes", "ws-1", "repo-a"]);
  });

  it("keeps workspaces distinct even with the same repo id", () => {
    expect(stashesQueryKey("ws-1", "repo-a")).not.toEqual(stashesQueryKey("ws-2", "repo-a"));
  });
});
