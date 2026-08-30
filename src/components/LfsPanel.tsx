// Git LFS panel — availability probe, per-repo install (`git lfs install
// --local`), and .gitattributes pattern management. libgit2 has no LFS
// support, so install shells out to the hidden `git lfs` CLI; patterns are
// plain .gitattributes edits.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, CircleAlert, Plus, X } from "lucide-react";
import { formatAppError, lfsInstall, lfsStatus, lfsTrack, lfsUntrack } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { useStatusAreaStore } from "@/stores/statusAreaStore";

export function LfsPanel({
  workspaceId,
  open,
  onClose,
}: {
  workspaceId: string;
  open: boolean;
  onClose: () => void;
}): React.JSX.Element {
  const [pattern, setPattern] = useState("");
  const [error, setError] = useState<string | null>(null);
  const setStatus = useStatusAreaStore((s) => s.setStatus);
  const queryClient = useQueryClient();

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ["lfs-status", workspaceId] });
    void queryClient.invalidateQueries({ queryKey: ["working-copy"] });
  };

  const { data: status } = useQuery({
    queryKey: ["lfs-status", workspaceId],
    queryFn: () => lfsStatus(workspaceId),
    enabled: open,
  });

  const installMut = useMutation({
    mutationFn: () => lfsInstall(workspaceId),
    onSuccess: () => {
      setStatus("LFS filters wired into this repository");
      refresh();
    },
    onError: (e) => setError(formatAppError(e)),
  });

  const trackMut = useMutation({
    mutationFn: () => {
      const trimmed = pattern.trim();
      if (!trimmed) throw new Error("enter a pattern first");
      return lfsTrack(workspaceId, trimmed);
    },
    onSuccess: () => {
      setPattern("");
      setError(null);
      refresh();
    },
    onError: (e) => setError(formatAppError(e)),
  });

  const untrackMut = useMutation({
    mutationFn: (p: string) => lfsUntrack(workspaceId, p),
    onSuccess: refresh,
    onError: (e) => setError(formatAppError(e)),
  });

  const busy = installMut.isPending || trackMut.isPending || untrackMut.isPending;

  return (
    <Modal
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title="Git LFS"
      description="Track large files with Git LFS. Patterns are stored in .gitattributes."
      size="sm"
    >
      <div className="flex flex-col gap-3">
        {status && !status.available ? (
          <p className="flex items-start gap-1.5 rounded-md bg-bg-secondary px-2 py-1.5 text-xs text-text-secondary">
            <CircleAlert size={14} className="mt-0.5 shrink-0 text-warning" />
            <span>
              <code>git lfs</code> was not found on PATH. Install Git LFS (
              <span className="font-mono">https://git-lfs.com</span>) to commit large files.
            </span>
          </p>
        ) : null}

        {status?.available ? (
          status.installed ? (
            <p className="flex items-center gap-1.5 text-xs text-success">
              <Check size={14} /> LFS filters are wired into this repository
            </p>
          ) : (
            <Button
              variant="secondary"
              size="sm"
              className="self-start"
              disabled={busy}
              onClick={() => installMut.mutate()}
            >
              Install LFS in this repository
            </Button>
          )
        ) : null}

        <div className="flex flex-col gap-2 rounded-xl bg-bg-primary p-3">
          <div className="flex flex-col gap-1">
            {status?.patterns.length ? (
              status.patterns.map((p) => (
                <div
                  key={p}
                  className="flex items-center gap-2 rounded-md border border-border-subtle px-2 py-1"
                >
                  <span className="min-w-0 flex-1 truncate font-mono text-xs">{p}</span>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="p-1 text-text-muted hover:text-danger"
                    aria-label={`Stop tracking ${p}`}
                    title="Stop tracking"
                    disabled={busy}
                    onClick={() => untrackMut.mutate(p)}
                  >
                    <X size={13} />
                  </Button>
                </div>
              ))
            ) : (
              <p className="text-xs text-text-muted italic">No LFS patterns tracked yet.</p>
            )}
          </div>

          <form
            className="flex items-center gap-1.5"
            onSubmit={(e) => {
              e.preventDefault();
              if (pattern.trim()) trackMut.mutate();
            }}
          >
            <Input
              value={pattern}
              onChange={setPattern}
              placeholder="Pattern, e.g. *.psd or assets/**"
              disabled={busy}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  if (pattern.trim()) trackMut.mutate();
                }
              }}
            />
            <Button type="submit" variant="secondary" size="sm" disabled={busy || !pattern.trim()}>
              <Plus size={13} />
              Track
            </Button>
          </form>
        </div>

        {error ? <p className="text-xs text-danger">{error}</p> : null}
      </div>
    </Modal>
  );
}
