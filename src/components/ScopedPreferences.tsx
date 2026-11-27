import { useTranslation } from "react-i18next";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { invoke } from "@tauri-apps/api/core";
import { isMacOS } from "@/lib/platform";
import { getWorkspace, formatAppError } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { paletteChords, usePaletteShortcut } from "@/stores/paletteShortcut";
export function ScopedPreferences() {
  const { t } = useTranslation();
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const shortcut = usePaletteShortcut();
  const client = useQueryClient();
  const workspace = useQuery({
    queryKey: ["workspace", workspaceId],
    queryFn: () => getWorkspace(workspaceId!),
    enabled: !!workspaceId,
  });
  const repo = workspace.data?.repos.find((r) => r.id === repoId);
  const write = useMutation({
    mutationFn: (args: { workspaceId: string; repoId: string; offline: boolean }) =>
      invoke<void>("cmd_set_repo_offline", args),
    onSuccess: async (_, args) => {
      await Promise.all([
        client.invalidateQueries({ queryKey: ["workspace", args.workspaceId] }),
        client.invalidateQueries({ queryKey: ["repos", args.workspaceId] }),
      ]);
    },
  });
  const ownWrite =
    write.variables?.workspaceId === workspaceId && write.variables?.repoId === repoId;
  const error = (ownWrite ? write.error : null) ?? workspace.error;
  return (
    <section className="space-y-3 py-3 text-sm">
      <label className="block">
        {t("settings.scoped.chord")}{" "}
        <select
          value={shortcut.chord}
          onChange={(e) => shortcut.set(paletteChords.find((c) => c === e.target.value) ?? "Mod+K")}
        >
          {paletteChords.map((c) => (
            <option key={c} value={c}>
              {c.replace("Mod", isMacOS() ? "⌘" : "Ctrl")}
            </option>
          ))}
        </select>
      </label>
      {workspaceId && repo && (
        <>
          <label className="block">
            {t("settings.scoped.offline")} · {repo.nickname || repo.path}{" "}
            <select
              disabled={write.isPending && ownWrite}
              value={repo.settings_override?.ai_offline ? "offline" : "inherit"}
              onChange={(e) =>
                write.mutate({
                  workspaceId,
                  repoId: repo.id,
                  offline: e.target.value === "offline",
                })
              }
            >
              <option value="inherit">{t("settings.scoped.inherit")}</option>
              <option value="offline">{t("settings.scoped.force")}</option>
            </select>
          </label>
          <p>
            {t(
              workspace.data?.settings.ai_offline || repo.settings_override?.ai_offline
                ? "settings.scoped.local"
                : "settings.scoped.online",
            )}
          </p>
        </>
      )}
      {error && <p role="alert">{formatAppError(error)}</p>}
    </section>
  );
}
