// F012: global auth-prompt state. A sync operation that failed on
// authentication registers a `retry` closure here; the single
// AuthPromptDialog (mounted in App) collects credentials and hands them
// back through `retry`. Registrars that await the outcome (withAuthRetry)
// also pass `onDismiss` so closing the dialog without submitting settles
// their operation as a user cancel instead of hanging forever.

import { create } from "zustand";

import type { InlineAuth } from "@/lib/api";

interface AuthPromptState {
  /** Remote name shown in the dialog, or null while closed. */
  remoteName: string | null;
  retry: ((auth: InlineAuth) => void) | null;
  /** Called when the dialog is closed without submitting. */
  onDismiss: (() => void) | null;
  show: (remoteName: string, retry: (auth: InlineAuth) => void, onDismiss?: () => void) => void;
  /** Clear state after a submit — the retry owns the outcome now. */
  close: () => void;
  /** Clear state after the user dismissed the dialog, notifying the registrar. */
  cancel: () => void;
}

export const useAuthPromptStore = create<AuthPromptState>((set, get) => ({
  remoteName: null,
  retry: null,
  onDismiss: null,
  show: (remoteName, retry, onDismiss) => {
    // The dialog is a global singleton, but nothing stops a second
    // operation from registering while the first one is still waiting.
    // Settle the previous registrant as cancelled first — dropping its
    // callbacks silently would leave its awaiting caller pending forever.
    get().cancel();
    set({ remoteName, retry, onDismiss: onDismiss ?? null });
  },
  close: () => set({ remoteName: null, retry: null, onDismiss: null }),
  cancel: () => {
    const { onDismiss } = get();
    get().close();
    onDismiss?.();
  },
}));
