import { create } from "zustand";

/** Failures belong to the repository, not the lifetime of its visible panel. */
export const useWorkingCopyErrorStore = create<{
  errors: Record<string, string | null>;
  setError: (scope: string, message: string | null) => void;
}>((set) => ({
  errors: {},
  setError: (scope, message) => set((state) => ({ errors: { ...state.errors, [scope]: message } })),
}));
