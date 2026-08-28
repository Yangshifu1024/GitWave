import { useEffect, useState } from "react";
import { FolderOpen, Github } from "lucide-react";
import { openUrl } from "@tauri-apps/plugin-opener";

import { formatAppError, getAppVersion, openDataDir } from "@/lib/api";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";

const GITHUB_URL = "https://github.com/Yangshifu1024/GitWave";
const SLOGAN = "Local-first Git client with AI collaboration.";

export interface AboutModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** Standalone About dialog: app identity, version and quick links. */
export function AboutModal({ open, onOpenChange }: AboutModalProps): React.JSX.Element {
  const [version, setVersion] = useState("…");
  const [actionError, setActionError] = useState<string | null>(null);

  // Fetch lazily on first open instead of at app startup.
  useEffect(() => {
    if (!open) return;
    getAppVersion()
      .then(setVersion)
      .catch(() => setVersion("?.?.?"));
  }, [open]);

  const handleOpenDataDir = async (): Promise<void> => {
    setActionError(null);
    try {
      await openDataDir();
    } catch (e) {
      setActionError(formatAppError(e));
    }
  };

  const handleOpenRepo = async (): Promise<void> => {
    setActionError(null);
    try {
      await openUrl(GITHUB_URL);
    } catch (e) {
      setActionError(formatAppError(e));
    }
  };

  return (
    <Modal open={open} onOpenChange={onOpenChange} title="About GitWave" size="sm">
      <div className="flex flex-col items-center justify-center gap-3 py-4 text-center">
        <img
          src="/app-icon.png"
          alt="GitWave app icon"
          draggable={false}
          className="size-20 rounded-xl"
        />
        <div>
          <p className="text-lg font-semibold text-text-primary">GitWave</p>
          <p className="text-sm text-text-muted tabular-nums">v{version}</p>
        </div>
        <p className="max-w-xs text-sm text-text-secondary">{SLOGAN}</p>
        <div className="mt-2 flex items-center gap-2">
          <Button variant="secondary" size="sm" onClick={() => void handleOpenDataDir()}>
            <FolderOpen size={14} />
            App Data
          </Button>
          <Button variant="secondary" size="sm" onClick={() => void handleOpenRepo()}>
            <Github size={14} />
            GitHub
          </Button>
        </div>
        {actionError ? <p className="text-xs text-danger">{actionError}</p> : null}
      </div>
    </Modal>
  );
}
