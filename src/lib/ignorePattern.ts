/**
 * Derive the ignore choices offered for a repo-relative file path.
 * All patterns are repo-root relative POSIX paths (git status format).
 */
export interface IgnorePatterns {
  /** The exact file path, e.g. `src/foo/bar.ts`. */
  full: string;
  /** Parent directory with trailing slash; undefined at repo root. */
  dir?: string;
  /** Extension wildcard, e.g. `*.ts`; undefined when there is no extension. */
  ext?: string;
}

/** Ignore options applicable to `path` (file name segments are `/`-separated). */
export function deriveIgnorePatterns(path: string): IgnorePatterns {
  const segments = path.split("/");
  const name = segments[segments.length - 1] ?? path;

  const patterns: IgnorePatterns = { full: path };

  const dir = segments.slice(0, -1).join("/");
  if (dir) patterns.dir = `${dir}/`;

  // Dotfiles like `.gitignore` have no extension ("lastIndexOf" > 0 guards).
  const dot = name.lastIndexOf(".");
  if (dot > 0) patterns.ext = `*${name.slice(dot)}`;

  return patterns;
}
