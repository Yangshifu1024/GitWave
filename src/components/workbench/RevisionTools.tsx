import { useState } from "react";
import { useQuery, useInfiniteQuery } from "@tanstack/react-query";
import { invoke } from "@tauri-apps/api/core";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { DiffText } from "@/components/DiffText";
import { getDiffPreview, formatAppError } from "@/lib/api";
type Page = { entries: [string, string][]; next: string | null };
export function RevisionTools({
  workspaceId,
  repoId,
  sha,
  path,
  onLocate,
}: {
  workspaceId: string;
  repoId: string;
  sha: string;
  path: string | null;
  onLocate: (oid: string) => void;
}) {
  const { t } = useTranslation();
  const [base, setBase] = useState("");
  const [mode, setMode] = useState<"compare" | "path" | null>(null);
  const [file, setFile] = useState("");
  const [expandedPath, setExpandedPath] = useState<string | null>(null);
  const pair: [string, string] = [base, sha];
  const compare = useQuery({
    queryKey: ["revision-compare", workspaceId, repoId, base, sha],
    queryFn: () => getDiffPreview(workspaceId, repoId, { compare: pair }),
    enabled: mode === "compare",
    retry: false,
  });
  const history = useInfiniteQuery({
    queryKey: ["path-history", workspaceId, repoId, sha, path],
    initialPageParam: sha,
    queryFn: ({ pageParam }) =>
      invoke<Page>("cmd_path_history", { workspaceId, repoId, start: pageParam, path }),
    getNextPageParam: (last) => last.next ?? undefined,
    enabled: mode === "path" && !!path,
    retry: false,
  });
  const files = compare.data?.diff.files;
  const selected = files?.find((f) => f.path === file) ?? files?.[0];
  const currentPath = JSON.stringify([base, sha, selected?.path]);
  const expanded = currentPath === expandedPath;
  const content = useQuery({
    queryKey: ["revision-file", workspaceId, repoId, base, sha, selected?.path, expanded],
    queryFn: () =>
      getDiffPreview(workspaceId, repoId, { compare: pair, path: selected!.path, expanded }),
    enabled: mode === "compare" && !!selected,
    retry: false,
  });
  const patch = content.data?.diff.files[0];
  const error = mode === "compare" ? (compare.error ?? content.error) : history.error;
  const retry = () => {
    if (mode === "compare") return void (compare.error ? compare : content).refetch();
    if (history.isFetchNextPageError) return void history.fetchNextPage();
    void history.refetch();
  };
  return (
    <div className="flex flex-wrap gap-1 border-b border-border-subtle">
      <Button size="sm" variant="ghost" onClick={() => setBase(sha)}>
        {t("app.revisions.setBase")}
      </Button>
      <Button
        size="sm"
        disabled={!base || base === sha}
        onClick={() => {
          setFile("");
          setExpandedPath(null);
          setMode("compare");
        }}
      >
        {t("app.revisions.compare")} {base.slice(0, 7)}
      </Button>
      <Button size="sm" disabled={!path} onClick={() => setMode("path")}>
        {t("app.revisions.path")}
      </Button>
      <Modal
        size="xl"
        open={!!mode}
        onOpenChange={(open) => {
          if (!open) setMode(null);
        }}
        title={t(mode === "compare" ? "app.revisions.compare" : "app.revisions.path")}
      >
        {error && (
          <p role="alert">
            {formatAppError(error)} <Button onClick={retry}>{t("common.retry")}</Button>
          </p>
        )}
        {mode === "compare" ? (
          <>
            <code>
              {base.slice(0, 12)} → {sha.slice(0, 12)}
            </code>
            {compare.isLoading && <p>{t("diff.preview.loading")}</p>}
            {files?.length === 0 && <p>{t("app.workbench.noFiles")}</p>}
            {selected && (
              <>
                <select
                  className="w-full"
                  aria-label={t("app.revisions.file")}
                  value={selected.path}
                  onChange={(e) => setFile(e.target.value)}
                >
                  {files?.map((f) => (
                    <option key={f.path}>{f.path}</option>
                  ))}
                </select>
                {content.isLoading && <p>{t("diff.preview.loading")}</p>}
                {content.data?.truncated && (
                  <p>
                    {t(content.data.too_large ? "diff.preview.tooLarge" : "diff.preview.truncated")}
                  </p>
                )}
                {content.data?.truncated && !expanded && (
                  <Button onClick={() => setExpandedPath(currentPath)}>
                    {t("diff.preview.loadMore")}
                  </Button>
                )}
                {patch &&
                  (patch.hunks.length ? (
                    <DiffText file={patch} mode="unified" />
                  ) : (
                    !content.data?.truncated && <p>{t("diff.preview.noText")}</p>
                  ))}
              </>
            )}
          </>
        ) : (
          <>
            <p>
              {t("app.revisions.pathHint")} <code>{path}</code>
            </p>
            <div className="max-h-[50vh] overflow-y-auto">
              {history.data?.pages
                .flatMap((page) => page.entries)
                .map(([oid, summary]) => (
                  <button
                    key={oid}
                    className="block text-left"
                    onClick={() => {
                      setMode(null);
                      onLocate(oid);
                    }}
                  >
                    {oid.slice(0, 7)} {summary}
                  </button>
                ))}
            </div>
            {history.isFetching && <p>{t("diff.preview.loading")}</p>}
            {history.data && !history.data.pages.some((p) => p.entries.length) && (
              <p>{t("app.revisions.noEntries")}</p>
            )}
            {history.hasNextPage && (
              <Button disabled={history.isFetching} onClick={() => void history.fetchNextPage()}>
                {t("palette.continueSearch")}
              </Button>
            )}
          </>
        )}
      </Modal>
    </div>
  );
}
