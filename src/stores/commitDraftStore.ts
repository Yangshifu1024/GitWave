import { create } from "zustand";

export interface CommitDraft {
  message: string;
  previousMessage?: string;
  amendHead?: string;
}

const EMPTY_DRAFT: CommitDraft = { message: "" };
export const commitDraftKey = (workspaceId: string | null, repoId: string | null): string =>
  JSON.stringify([workspaceId, repoId]);

/** Session-only: closing a modal must not discard potentially sensitive input. */
export const useCommitDraftStore = create<{
  drafts: Record<string, CommitDraft>;
  update: (key: string, patch: Partial<CommitDraft>) => void;
  clear: (key: string) => void;
}>((set) => ({
  drafts: {},
  update: (key, patch) =>
    set((state) => ({
      drafts: { ...state.drafts, [key]: { ...(state.drafts[key] ?? EMPTY_DRAFT), ...patch } },
    })),
  clear: (key) =>
    set((state) => {
      const drafts = { ...state.drafts };
      delete drafts[key];
      return { drafts };
    }),
}));

export const getCommitDraft = (drafts: Record<string, CommitDraft>, key: string): CommitDraft =>
  drafts[key] ?? EMPTY_DRAFT;
