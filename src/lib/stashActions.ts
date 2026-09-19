// Pure helpers behind the stash panel UI. They live in lib/ (not in the
// components) so the derivations stay unit-testable: a FileDiff carries no
// `kind` field, and the drop dialog only ever shows an excerpt of the names.

import type { FileDiff, FileStatusKind, StashEntry } from "@/lib/api";

/** `stash@{n}` — the label git itself uses, so it matches what users type. */
export function stashLabel(entry: StashEntry): string {
  return `stash@{${entry.index}}`;
}

/** Dialog / list title: the label plus the message (or the "no message" text). */
export function stashTitle(entry: StashEntry, noMessageLabel: string): string {
  return `${stashLabel(entry)} · ${entry.message || noMessageLabel}`;
}

/** FileDiff has no status field: derive it from which side of the diff exists. */
export function fileDiffKind(f: FileDiff): FileStatusKind {
  // Untracked files come from the stash's third parent, so their old side is
  // empty and the derivation below would call them "added" while the row badges
  // them "untracked" (`?` icon). The explicit flag wins.
  if (f.untracked === true) return "untracked";
  if (f.old_sha === null) return "added";
  if (f.new_sha === null) return "deleted";
  return "modified";
}

/** File count + a truncated name excerpt for the drop confirmation. */
export function stashFileSummary(
  files: FileDiff[],
  maxNames = 5,
): { count: number; names: string[] } {
  return { count: files.length, names: files.slice(0, maxNames).map((f) => f.path) };
}
