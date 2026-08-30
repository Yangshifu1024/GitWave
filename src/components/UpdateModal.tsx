import { openUrl } from "@tauri-apps/plugin-opener";
import { Github } from "lucide-react";
import { useTranslation } from "react-i18next";
import { ProgressBar } from "@heroui/react";

import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import {
  useInstallUpdate,
  useOpenReleases,
  useRestartApp,
  useRetryUpdate,
} from "@/hooks/useUpdater";
import { useUpdaterStore } from "@/stores/updaterStore";

function releaseNotesUrl(version: string): string {
  return `https://github.com/Yangshifu1024/GitWave/releases/tag/v${version}`;
}

function formatBytes(bytes: number): string {
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${bytes} B`;
}

/**
 * Self-update surface: opened on startup / manual check when a newer release
 * exists. Install path by phase — in-app download + relaunch, or a jump to
 * the releases page for deb/rpm installs.
 */
export function UpdateModal(): React.JSX.Element {
  const { t } = useTranslation();
  const phase = useUpdaterStore((s) => s.phase);
  const modalOpen = useUpdaterStore((s) => s.modalOpen);
  const setModalOpen = useUpdaterStore((s) => s.setModalOpen);
  const currentVersion = useUpdaterStore((s) => s.currentVersion);
  const newVersion = useUpdaterStore((s) => s.newVersion);
  const downloadedBytes = useUpdaterStore((s) => s.downloadedBytes);
  const totalBytes = useUpdaterStore((s) => s.totalBytes);
  const error = useUpdaterStore((s) => s.error);
  const install = useInstallUpdate();
  const restart = useRestartApp();
  const openReleases = useOpenReleases();
  const retry = useRetryUpdate();

  const percent =
    totalBytes && totalBytes > 0
      ? Math.min(100, Math.round((downloadedBytes / totalBytes) * 100))
      : null;

  const handleOpenNotes = (): void => {
    if (!newVersion) return;
    void openUrl(releaseNotesUrl(newVersion));
  };

  const footer = (() => {
    switch (phase) {
      case "available":
        return (
          <>
            <Button variant="secondary" size="sm" onClick={() => setModalOpen(false)}>
              {t("updater.actions.later")}
            </Button>
            <Button variant="primary" size="sm" onClick={install}>
              {t("updater.actions.downloadInstall")}
            </Button>
          </>
        );
      case "manual-download":
        return (
          <>
            <Button variant="secondary" size="sm" onClick={() => setModalOpen(false)}>
              {t("updater.actions.later")}
            </Button>
            <Button variant="primary" size="sm" onClick={openReleases}>
              {t("updater.actions.openReleases")}
            </Button>
          </>
        );
      case "downloading":
        return (
          <Button variant="secondary" size="sm" onClick={() => setModalOpen(false)}>
            {t("updater.actions.hide")}
          </Button>
        );
      case "ready":
        return (
          <>
            <Button variant="secondary" size="sm" onClick={() => setModalOpen(false)}>
              {t("updater.actions.later")}
            </Button>
            <Button variant="primary" size="sm" onClick={restart}>
              {t("updater.actions.restartNow")}
            </Button>
          </>
        );
      case "error":
        return (
          <>
            <Button variant="secondary" size="sm" onClick={() => setModalOpen(false)}>
              {t("updater.actions.close")}
            </Button>
            <Button variant="primary" size="sm" onClick={retry}>
              {t("updater.actions.tryAgain")}
            </Button>
          </>
        );
      default:
        return null;
    }
  })();

  return (
    <Modal
      open={modalOpen}
      onOpenChange={setModalOpen}
      title={
        phase === "ready"
          ? t("updater.title.installed")
          : phase === "error"
            ? t("updater.title.check")
            : t("updater.title.available")
      }
      size="sm"
      footer={footer}
    >
      <div className="flex min-w-0 flex-col gap-3 py-1">
        <div className="flex flex-col gap-0.5">
          <p className="text-sm text-text-primary">
            GitWave <span className="font-medium tabular-nums">v{newVersion ?? "?"}</span>
            {t("updater.availableSuffix")}
          </p>
          <p className="text-xs text-text-muted tabular-nums">
            {t("updater.runningVersion", { version: currentVersion ?? "?.?.?" })}
          </p>
        </div>

        {newVersion ? (
          <Button variant="ghost" size="sm" className="self-start" onClick={handleOpenNotes}>
            <Github size={14} />
            {t("updater.viewReleaseNotes")}
          </Button>
        ) : null}

        {phase === "manual-download" ? (
          <p className="text-xs text-text-secondary">{t("updater.manualDownloadNote")}</p>
        ) : null}

        {phase === "downloading" ? (
          <div className="flex flex-col gap-1.5">
            <ProgressBar
              aria-label={t("updater.downloadingAria")}
              minValue={0}
              maxValue={100}
              value={percent ?? 0}
            >
              <ProgressBar.Track className="h-1.5 rounded-full bg-accent/25 overflow-hidden">
                <ProgressBar.Fill className="h-full rounded-full bg-accent" />
              </ProgressBar.Track>
            </ProgressBar>
            <p className="text-xs text-text-muted tabular-nums">
              {formatBytes(downloadedBytes)}
              {totalBytes ? ` / ${formatBytes(totalBytes)}` : ""}
            </p>
          </div>
        ) : null}

        {phase === "ready" ? (
          <p className="text-xs text-text-secondary">{t("updater.restartToApply")}</p>
        ) : null}

        {phase === "error" && error ? (
          <p className="text-xs text-danger break-words">{error}</p>
        ) : null}
      </div>
    </Modal>
  );
}
