import { useState } from "react";
import { Search } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { useCommitPages } from "@/hooks/useCommitPages";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

export function HistorySearch({
  workspaceId,
  repoId,
  onLocate,
}: {
  workspaceId: string;
  repoId: string;
  onLocate: (sha: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const epoch = useWorkspaceUiStore((s) => s.historyEpoch);
  const filter = query.trim();
  const results = useCommitPages({
    workspaceId,
    repoId,
    epoch,
    filter,
    enabled: open && !!filter,
    debounceMs: 250,
    pageSize: 30,
  });
  const close = () => {
    setOpen(false);
    setQuery("");
  };
  return (
    <>
      <Button size="sm" variant="ghost" onClick={() => setOpen(true)}>
        <Search size={14} />
        {t("app.workbench.search")}
      </Button>
      <Modal
        open={open}
        onOpenChange={(next) => {
          if (!next) close();
        }}
        title={t("app.workbench.search")}
      >
        <Input
          autoFocus
          variant="search"
          aria-label={t("app.workbench.search")}
          placeholder={t("app.workbench.searchHint")}
          value={query}
          onChange={setQuery}
        />
        <div className="max-h-[45vh] overflow-auto">
          {results.commits.map((c) => (
            <button
              type="button"
              key={c.sha}
              className="workbench-file text-left"
              onClick={() => {
                close();
                onLocate(c.sha);
              }}
            >
              <code className="text-text-muted">{c.sha.slice(0, 7)}</code>
              <span className="truncate">{c.message_summary}</span>
            </button>
          ))}
        </div>
        {results.loading && <p role="status">{t("palette.searchingCommits")}</p>}
        {results.error && (
          <div role="alert">
            {results.error} <Button onClick={results.retry}>{t("common.retry")}</Button>
          </div>
        )}
        {filter && !results.loading && !results.error && !results.commits.length && (
          <p role="status">{t("app.workbench.noSearchResults")}</p>
        )}
        {results.snapshotSize > 0 && (
          <p className="text-xs text-text-muted">
            {t("palette.searchProgress", { scanned: results.scanned, total: results.snapshotSize })}
          </p>
        )}
        {results.snapshotTruncated && (
          <p className="text-xs text-text-muted">
            {t("app.workbench.searchLimit", { count: results.snapshotSize })}
          </p>
        )}
        {results.hasMore && !results.error && (
          <Button disabled={results.loading} onClick={results.loadMore}>
            {t("palette.continueSearch")}
          </Button>
        )}
      </Modal>
    </>
  );
}
