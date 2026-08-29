import { create } from "zustand";

export type StatusVariant = "success" | "danger" | "info";

export interface StatusEntry {
  text: string;
  variant: StatusVariant;
}

interface StatusAreaState {
  /** Last operation result; persists until the next operation overwrites it. */
  status: StatusEntry | null;
  setStatus: (text: string, variant?: StatusVariant) => void;
  clearStatus: () => void;
}

export const useStatusAreaStore = create<StatusAreaState>((set) => ({
  status: null,
  setStatus: (text, variant = "success") => set({ status: { text, variant } }),
  clearStatus: () => set({ status: null }),
}));
