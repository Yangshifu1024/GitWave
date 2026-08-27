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

/** When `path` is set, keep only that file. `staged` further splits working-copy sides. */
export function filterDiffSummary(
  diff: DiffSummary,
  path?: string | null,
  staged?: boolean | null,
): DiffSummary {
  if (!path && staged == null) return diff;
  const needle = path ? normalizeRepoPath(path) : null;
  const files = diff.files.filter((file) => {
    if (needle && normalizeRepoPath(file.path) !== needle) return false;
    if (staged != null && file.staged !== staged) return false;
    return true;
  });
  return {
    files,
    total_additions: files.reduce((sum, file) => sum + file.additions, 0),
    total_deletions: files.reduce((sum, file) => sum + file.deletions, 0),
  };
}

function normalizeRepoPath(path: string): string {
  return path.replace(/\\/g, "/");
}
