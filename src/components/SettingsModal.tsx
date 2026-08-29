import { useEffect, useState } from "react";
import { KeyRound, Monitor, Moon, Palette as PaletteIcon, Sun } from "lucide-react";

import { usePalette } from "@/hooks/usePalette";
import { useTheme, type Theme } from "@/hooks/useTheme";
import { PALETTE_META, type Palette } from "@/lib/palette";
import { Radio, RadioGroup } from "@heroui/react";
import { Modal } from "@/components/ui/Modal";
import { SshKeyManager } from "@/components/SshKeyManager";
import { Button } from "@/components/ui/Button";
import { cn } from "@/lib/utils";

type SettingsSection = "appearance" | "ssh";

const SECTIONS: { id: SettingsSection; label: string; icon: typeof PaletteIcon }[] = [
  { id: "appearance", label: "Appearance", icon: PaletteIcon },
  { id: "ssh", label: "SSH Keys", icon: KeyRound },
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
            <CardOption key={id} value={id} selected={palette === id} label={PALETTE_META[id].name}>
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
      className="flex h-7 w-20 shrink-0 items-center gap-1.5 rounded-md border border-black/8 p-1"
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

/** Card-style radio option: preview slot and label side by side, single row. */
function CardOption({
  value,
  selected,
  label,
  children,
}: {
  value: string;
  selected: boolean;
  label: string;
  children: React.ReactNode;
}): React.JSX.Element {
  return (
    <Radio value={value} className="w-full">
      <Radio.Content
        className={cn(
          "flex h-auto w-full flex-row items-center gap-2 rounded-lg border p-2 text-left transition-colors",
          selected
            ? "border-accent bg-accent/5 ring-accent/30 ring-1"
            : "border-border-default hover:border-border-strong",
        )}
      >
        {children}
        <span
          className={cn(
            "block truncate text-sm",
            selected ? "font-medium text-text-primary" : "text-text-secondary",
          )}
        >
          {label}
        </span>
      </Radio.Content>
    </Radio>
  );
}
