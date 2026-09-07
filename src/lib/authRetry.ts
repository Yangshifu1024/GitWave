// F012 retry wrapper for imperative (non-mutation-hook) sync flows —
// branch deletes, submodule update/add, clone: run `fn`; when it fails on
// authentication, open the in-app auth prompt once. Submitting re-runs
// `fn` with the entered credentials and settles with that outcome; closing
// the prompt rejects with a sync-cancelled error so callers treat the
// dismissal like any other user abort instead of hanging. A retry that
// fails auth again surfaces as a plain error — at most one prompt.

import { isAuthError, SYNC_CANCELLED_CODE, type InlineAuth } from "@/lib/api";
import { useAuthPromptStore } from "@/stores/authPromptStore";

/** Front-end stand-in for an auth prompt the user dismissed; carries the
 *  backend's cancel code so existing isCancelledSyncError guards match. */
export function authPromptCancelledError(): Error & { code: string } {
  const err = new Error("authentication prompt dismissed") as Error & { code: string };
  err.code = SYNC_CANCELLED_CODE;
  return err;
}

/** Best display name for a URL-based remote: its host (scp-style URLs and
 *  garbage fall back to the raw input). */
export function remoteHost(url: string): string {
  try {
    return new URL(url).host || url;
  } catch {
    return url;
  }
}

export function withAuthRetry(
  remote: string,
  fn: (auth?: InlineAuth) => Promise<void>,
): Promise<void> {
  return fn().catch((e: unknown) => {
    if (!isAuthError(e)) throw e;
    return new Promise<void>((resolve, reject) => {
      useAuthPromptStore.getState().show(
        remote,
        (auth) => {
          fn(auth).then(resolve, reject);
        },
        () => reject(authPromptCancelledError()),
      );
    });
  });
}
