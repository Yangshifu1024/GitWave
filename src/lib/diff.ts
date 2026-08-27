import type { DiffSummary, FileChange } from "@/lib/api";

export function partitionFileChanges(files: FileChange[]): {
  unstaged: FileChange[];
  staged: FileChange[];
} {
  const unstaged: FileChange[] = [];
  const staged: FileChange[] = [];
  for (const file of files) {
    if (file.staged) staged.push(file);
    else unstaged.push(file);
  }
  return { unstaged, staged };
}

/** When `path` is set, keep only that file and recompute totals. */
export function filterDiffSummary(diff: DiffSummary, path?: string | null): DiffSummary {
  if (!path) return diff;
  const files = diff.files.filter((file) => file.path === path);
  return {
    files,
    total_additions: files.reduce((sum, file) => sum + file.additions, 0),
    total_deletions: files.reduce((sum, file) => sum + file.deletions, 0),
  };
}
