import type { BundledLanguage } from "shiki";
import { useEffect, useMemo, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import type { DiffLine, FileDiff } from "@/lib/api";
import { changedSpan, diffRows } from "@/lib/diffRows";
import { cn } from "@/lib/utils";

type Token = { content: string; color?: string };
const languages: Record<string, BundledLanguage> = {
  ts: "typescript",
  tsx: "tsx",
  js: "javascript",
  jsx: "jsx",
  rs: "rust",
  py: "python",
  go: "go",
  java: "java",
  c: "c",
  cpp: "cpp",
  json: "json",
  md: "markdown",
  toml: "toml",
  yml: "yaml",
  yaml: "yaml",
  sh: "bash",
};

function useSyntax(file: FileDiff): Map<DiffLine, Token[]> {
  const [tokens, setTokens] = useState(new Map<DiffLine, Token[]>());
  useEffect(() => {
    let cancelled = false;
    const lines = file.hunks.flatMap((h) => h.lines);
    setTokens(new Map());
    // Large previews remain plain text: tokenization must not monopolize the UI.
    if (
      lines.length > 2000 ||
      lines.reduce((n, l) => n + l.content.replace(/\t/g, "    ").length, 0) > 100_000
    )
      return;
    const lang = languages[file.path.split(".").pop() ?? ""];
    if (!lang) return;
    const highlight = async () => {
      const { codeToTokens } = await import("shiki");
      const theme =
        document.documentElement.dataset.theme === "dark" ? "github-dark" : "github-light";
      const result = new Map<DiffLine, Token[]>();
      for (const side of ["removed", "added"] as const) {
        const source = lines.filter((l) => l.kind === "context" || l.kind === side);
        const highlighted = await codeToTokens(source.map((l) => l.content).join("\n"), {
          lang,
          theme,
        });
        source.forEach((line, i) => result.set(line, highlighted.tokens[i] ?? []));
      }
      if (!cancelled) setTokens(result);
    };
    void highlight().catch(() => {
      /* Unknown grammar falls back to readable text. */
    });
    const observer = new MutationObserver(() => {
      void highlight().catch(() => {});
    });
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["data-theme"],
    });
    return () => {
      cancelled = true;
      observer.disconnect();
    };
  }, [file]);
  return tokens;
}

function Text({
  line,
  other,
  side,
  tokens,
}: {
  line: DiffLine;
  other?: DiffLine;
  side: "left" | "right";
  tokens?: Token[];
}): React.JSX.Element {
  const span =
    other && line.kind !== "context"
      ? changedSpan(
          side === "left" ? line.content : other.content,
          side === "right" ? line.content : other.content,
          side,
        )
      : null;
  let offset = 0;
  return (
    <>
      {(tokens ?? [{ content: line.content }]).map((token, i) => {
        const start = offset;
        offset += token.content.length;
        const a = span ? Math.max(start, span[0]) - start : 0;
        const b = span ? Math.min(offset, span[1]) - start : 0;
        return (
          <span key={i} style={{ color: token.color }}>
            {b > a ? (
              <>
                {token.content.slice(0, a)}
                <mark
                  className={cn(
                    "text-inherit",
                    side === "left" ? "bg-diff-del-word" : "bg-diff-add-word",
                  )}
                >
                  {token.content.slice(a, b)}
                </mark>
                {token.content.slice(b)}
              </>
            ) : (
              token.content
            )}
          </span>
        );
      })}
    </>
  );
}

/** A bounded viewport renders only visible lines even for expanded previews. */
export function DiffText({
  file,
  mode,
}: {
  file: FileDiff;
  mode: "split" | "unified";
}): React.JSX.Element {
  const parent = useRef<HTMLDivElement>(null);
  const rows = useMemo(() => diffRows(file.hunks, mode === "split"), [file.hunks, mode]);
  const tokens = useSyntax(file);
  const virtual = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parent.current,
    estimateSize: () => 20,
    overscan: 12,
  });
  const width = useMemo(
    () =>
      file.hunks.reduce(
        (max, h) =>
          h.lines.reduce((n, l) => Math.max(n, l.content.replace(/\t/g, "        ").length), max),
        0,
      ) *
        16 +
      100,
    [file.hunks],
  );
  const cell = (
    line: DiffLine | undefined,
    other: DiffLine | undefined,
    side: "left" | "right",
  ) => (
    <div
      className={cn(
        "flex min-w-0 overflow-hidden",
        line?.kind === "removed" && "bg-diff-del-bg",
        line?.kind === "added" && "bg-diff-add-bg",
      )}
    >
      <span className="w-12 shrink-0 bg-bg-elevated text-right pr-2 text-text-muted select-none">
        {side === "left" ? line?.old_line_no : line?.new_line_no}
      </span>
      <span className="whitespace-pre px-2">
        {line ? <Text line={line} other={other} side={side} tokens={tokens.get(line)} /> : ""}
      </span>
    </div>
  );
  return (
    <div
      ref={parent}
      className="overflow-auto font-mono text-xs leading-5"
      style={{ height: Math.min(560, Math.max(40, rows.length * 20)) }}
    >
      <div
        style={{
          height: virtual.getTotalSize(),
          minWidth: "100%",
          width: mode === "split" ? width * 2 : width,
          position: "relative",
        }}
      >
        {virtual.getVirtualItems().map((item) => {
          const row = rows[item.index]!;
          return (
            <div
              key={item.key}
              style={{
                position: "absolute",
                top: 0,
                width: "100%",
                height: 20,
                transform: `translateY(${item.start}px)`,
              }}
            >
              {"header" in row ? (
                <div className="bg-diff-hunk-bg px-2 text-text-muted">{row.header}</div>
              ) : mode === "split" ? (
                <div className="grid grid-cols-2 divide-x divide-border-subtle">
                  {cell(row.left, row.right, "left")}
                  {cell(row.right, row.left, "right")}
                </div>
              ) : (
                cell(
                  row.side === "right" ? row.right : row.left,
                  row.side === "right" ? row.left : row.right,
                  row.side ?? "left",
                )
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
