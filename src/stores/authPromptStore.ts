// F012: global auth-prompt state. A sync operation that failed on
// authentication registers a `retry` closure here; the single
// AuthPromptDialog (mounted in App) collects credentials and hands them
// back through `retry`.

import { create } from "zustand";

import type { InlineAuth } from "@/lib/api";

interface AuthPromptState {
  /** Remote name shown in the dialog, or null while closed. */
  remoteName: string | null;
  retry: ((auth: InlineAuth) => void) | null;
  show: (remoteName: string, retry: (auth: InlineAuth) => void) => void;
  close: () => void;
}

export const useAuthPromptStore = create<AuthPromptState>((set) => ({
  remoteName: null,
  retry: null,
  show: (remoteName, retry) => set({ remoteName, retry }),
  close: () => set({ remoteName: null, retry: null }),
}));
