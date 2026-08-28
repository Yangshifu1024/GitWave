import { useEffect, useState } from "react";
import {
  FolderOpen,
  Github,
  Info,
  KeyRound,
  Monitor,
  Moon,
  Palette as PaletteIcon,
  Sun,
} from "lucide-react";
import { openUrl } from "@tauri-apps/plugin-opener";

import { usePalette } from "@/hooks/usePalette";
import { useTheme, type Theme } from "@/hooks/useTheme";
import { PALETTE_META, type Palette } from "@/lib/palette";
import { formatAppError, getAppVersion, openDataDir } from "@/lib/api";
import { Radio, RadioGroup } from "@heroui/react";
import { Modal } from "@/components/ui/Modal";
import { SshKeyManager } from "@/components/SshKeyManager";
import { Button } from "@/components/ui/Button";
import { cn } from "@/lib/utils";

type SettingsSection = "appearance" | "ssh" | "about";

const GITHUB_URL = "https://github.com/Yangshifu1024/GitWave";
const SLOGAN = "Local-first Git client with AI collaboration.";

const SECTIONS: { id: SettingsSection; label: string; icon: typeof PaletteIcon }[] = [
  { id: "appearance", label: "Appearance", icon: PaletteIcon },
  { id: "ssh", label: "SSH Keys", icon: KeyRound },
  { id: "about", label: "About", icon: Info },
];

interface SettingsModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function SettingsModal({ open, onOpenChange }: SettingsModalProps): React.JSX.Element {
  const [section, setSection] = useState<SettingsSection>("appearance");

  // Reopening always lands on the first section instead of resuming the
  // previous one.
  useEffect(() => {
    if (open) setSection("appearance");
  }, [open]);

  return (
    <Modal open={open} onOpenChange={onOpenChange} title="Settings" size="lg">
      <div className="flex h-[60vh] min-h-0">
        <nav
          aria-label="Settings sections"
          className="flex w-44 shrink-0 flex-col gap-0.5 border-r border-border-subtle pr-2"
        >
          {SECTIONS.map(({ id, label, icon: Icon }) => (
            <Button
              key={id}
              type="button"
              variant="ghost"
              size="sm"
              aria-current={section === id ? "page" : undefined}
              onClick={() => setSection(id)}
              className={cn(
                "h-auto justify-start flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors duration-base",
                "focus-visible:outline-2 focus-visible:outline-accent focus-visible:outline-offset-1",
                section === id
                  ? "bg-accent/10 font-medium text-accent"
                  : "text-text-secondary hover:bg-accent/5 hover:text-text-primary",
              )}
            >
              <Icon size={15} className="shrink-0" />
              {label}
            </Button>
          ))}
        </nav>

        <div className="min-w-0 flex-1 overflow-auto pl-5">
          {section === "appearance" ? <AppearanceSection /> : null}
          {section === "ssh" ? <SshKeyManager /> : null}
          {section === "about" ? <AboutSection /> : null}
        </div>
      </div>
    </Modal>
  );
}

// ─── Appearance ───────────────────────────────────────────────────────────

const THEME_OPTIONS: { value: Theme; label: string; icon: typeof Sun }[] = [
  { value: "light", label: "Light", icon: Sun },
  { value: "dark", label: "Dark", icon: Moon },
  { value: "system", label: "System", icon: Monitor },
];

function AppearanceSection(): React.JSX.Element {
  const { theme, setTheme } = useTheme();
  const { palette, setPalette } = usePalette();

  return (
    <div className="flex flex-col gap-5">
      <section className="flex flex-col gap-2">
        <h3 className="text-xs font-medium uppercase tracking-wide text-text-muted">Theme</h3>
        <RadioGroup
          value={theme}
          onChange={(value) => setTheme(value as Theme)}
          aria-label="Theme"
          className="grid grid-cols-3 gap-2 [&_[data-slot=radio]]:mt-0"
          style={{ display: "grid" }}
        >
          {THEME_OPTIONS.map(({ value, label, icon: Icon }) => (
            <CardOption key={value} value={value} selected={theme === value} label={label}>
              <Icon size={18} className={theme === value ? "text-accent" : "text-text-secondary"} />
            </CardOption>
          ))}
        </RadioGroup>
      </section>

      <section className="flex flex-col gap-2">
        <h3 className="text-xs font-medium uppercase tracking-wide text-text-muted">
          Color palette
        </h3>
        <RadioGroup
          value={palette}
          onChange={(value) => setPalette(value as Palette)}
          aria-label="Color palette"
          className="grid grid-cols-2 gap-2 [&_[data-slot=radio]]:mt-0"
          style={{ display: "grid" }}
        >
          {(Object.keys(PALETTE_META) as Palette[]).map((id) => (
            <CardOption
              key={id}
              value={id}
              selected={palette === id}
              label={PALETTE_META[id].name}
              description={PALETTE_META[id].description}
            >
              <PaletteSwatch meta={PALETTE_META[id]} />
            </CardOption>
          ))}
        </RadioGroup>
      </section>
    </div>
  );
}

function PaletteSwatch({ meta }: { meta: (typeof PALETTE_META)[Palette] }): React.JSX.Element {
  return (
    <span
      aria-hidden="true"
      className="mb-2 flex h-10 items-center gap-1.5 rounded-md border border-black/8 p-1.5"
      style={{ backgroundColor: meta.swatch.canvas }}
    >
      <span
        className="h-full w-3 shrink-0 rounded-sm"
        style={{ backgroundColor: meta.swatch.sidebar }}
      />
      <span className="flex min-w-0 flex-1 items-center gap-1">
        {meta.swatch.lanes.map((lane) => (
          <span key={lane} className="h-1 flex-1 rounded-full" style={{ backgroundColor: lane }} />
        ))}
      </span>
      <span
        className="size-3 shrink-0 rounded-full border border-black/10"
        style={{ backgroundColor: meta.swatch.accent }}
      />
    </span>
  );
}

/** Card-style radio option with a preview slot, label and optional description. */
function CardOption({
  value,
  selected,
  label,
  description,
  children,
}: {
  value: string;
  selected: boolean;
  label: string;
  description?: string;
  children: React.ReactNode;
}): React.JSX.Element {
  return (
    <Radio value={value} className="w-full">
      <Radio.Content
        className={cn(
          "flex h-auto w-full flex-col items-start rounded-lg border p-2.5 text-left transition-colors",
          selected
            ? "border-accent bg-accent/5 ring-accent/30 ring-1"
            : "border-border-default hover:border-border-strong",
        )}
      >
        {children}
        <span
          className={cn(
            "block text-sm",
            selected ? "font-medium text-text-primary" : "text-text-secondary",
          )}
        >
          {label}
        </span>
        {description ? (
          <span className="mt-0.5 block text-xs leading-snug text-text-secondary">
            {description}
          </span>
        ) : null}
      </Radio.Content>
    </Radio>
  );
}

// ─── About ────────────────────────────────────────────────────────────────

function AboutSection(): React.JSX.Element {
  const [version, setVersion] = useState("…");
  const [actionError, setActionError] = useState<string | null>(null);

  useEffect(() => {
    getAppVersion()
      .then(setVersion)
      .catch(() => setVersion("?.?.?"));
  }, []);

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
    <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
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
  );
}
