import type { DiffSummary, FileChange } from "@/lib/api";
import { isWindows } from "@/lib/platform";

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
  // Only Windows treats `\` as a separator — on POSIX a literal backslash
  // is a legal filename character and must survive verbatim.
  return isWindows() ? path.replace(/\\/g, "/") : path;
}

/** Image MIME types the diff view can render side-by-side (F016). */
const IMAGE_MIME_BY_EXT: Record<string, string> = {
  png: "image/png",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  gif: "image/gif",
  webp: "image/webp",
  bmp: "image/bmp",
  ico: "image/x-icon",
  svg: "image/svg+xml",
};

/** Whether the diff view renders `path` as an image compare (by extension). */
export function isImagePath(path: string): boolean {
  const dot = path.lastIndexOf(".");
  // `dot <= 0` also excludes dotfiles like `.png` (no real extension).
  if (dot <= 0) return false;
  return path.slice(dot + 1).toLowerCase() in IMAGE_MIME_BY_EXT;
}

/** MIME type for an image diff path; only meaningful when `isImagePath` holds. */
export function imageMimeFromPath(path: string): string {
  const dot = path.lastIndexOf(".");
  const ext = dot > 0 ? path.slice(dot + 1).toLowerCase() : "";
  return IMAGE_MIME_BY_EXT[ext] ?? "application/octet-stream";
}
