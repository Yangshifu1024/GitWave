import { afterEach, describe, expect, it, vi } from "vitest";

import { isCancelledSyncError, SYNC_CANCELLED_CODE, type InlineAuth } from "@/lib/api";
import { useAuthPromptStore } from "@/stores/authPromptStore";

import { authPromptCancelledError, remoteHost, withAuthRetry } from "./authRetry";

function authError(): Error & { code: string } {
  const err = new Error("auth failed") as Error & { code: string };
  err.code = "git.fetch_auth_failed";
  return err;
}

function submitPrompt(auth: InlineAuth): void {
  const { retry, close } = useAuthPromptStore.getState();
  if (!retry) throw new Error("no auth prompt is waiting");
  close();
  retry(auth);
}

async function waitForPrompt(): Promise<void> {
  await vi.waitFor(() => {
    if (!useAuthPromptStore.getState().retry) throw new Error("prompt not open yet");
  });
}

afterEach(() => {
  useAuthPromptStore.getState().close();
});

describe("withAuthRetry", () => {
  it("resolves without prompting when the operation succeeds", async () => {
    let calls = 0;
    await withAuthRetry("origin", () => {
      calls += 1;
      return Promise.resolve();
    });

    expect(calls).toBe(1);
    expect(useAuthPromptStore.getState().retry).toBeNull();
  });

  it("retries once with the credentials submitted in the prompt", async () => {
    let seen: InlineAuth | undefined;
    const op = (auth?: InlineAuth): Promise<void> => {
      if (!auth) return Promise.reject(authError());
      seen = auth;
      return Promise.resolve();
    };

    const promise = withAuthRetry("origin", op);
    await waitForPrompt();
    expect(useAuthPromptStore.getState().remoteName).toBe("origin");
    submitPrompt({ username: "u", password: "p", remember: false });

    await promise;
    expect(seen).toEqual({ username: "u", password: "p", remember: false });
  });

  it("rejects with the cancel code when the prompt is dismissed", async () => {
    const promise = withAuthRetry("origin", () => Promise.reject(authError()));

    await waitForPrompt();
    useAuthPromptStore.getState().cancel();

    const err: unknown = await promise.then(
      () => null,
      (e: unknown) => e,
    );
    expect(isCancelledSyncError(err)).toBe(true);
  });

  it("rethrows non-auth errors without opening the prompt", async () => {
    const boom = new Error("network down");
    const promise = withAuthRetry("origin", () => Promise.reject(boom));

    await expect(promise).rejects.toBe(boom);
    expect(useAuthPromptStore.getState().retry).toBeNull();
  });

  it("does not prompt a second time when the retry fails auth again", async () => {
    const promise = withAuthRetry("origin", () => Promise.reject(authError()));

    await waitForPrompt();
    submitPrompt({ username: "u", password: "p", remember: false });

    const err: unknown = await promise.then(
      () => null,
      (e: unknown) => e,
    );
    expect(err).toBeInstanceOf(Error);
    expect(isCancelledSyncError(err)).toBe(false);
    // The prompt consumed its one shot: nothing is waiting afterwards.
    expect(useAuthPromptStore.getState().retry).toBeNull();
  });

  it("authPromptCancelledError carries the shared cancel code", () => {
    expect(isCancelledSyncError(authPromptCancelledError())).toBe(true);
    expect(authPromptCancelledError().code).toBe(SYNC_CANCELLED_CODE);
  });
});

describe("remoteHost", () => {
  it("returns the host of a normal URL", () => {
    expect(remoteHost("https://git.example.com/user/repo.git")).toBe("git.example.com");
  });

  it("keeps the port", () => {
    expect(remoteHost("http://localhost:3000/repo")).toBe("localhost:3000");
  });

  it("falls back to the raw input for scp-style or invalid values", () => {
    expect(remoteHost("git@github.com:user/repo.git")).toBe("git@github.com:user/repo.git");
    expect(remoteHost("not a url")).toBe("not a url");
  });
});
