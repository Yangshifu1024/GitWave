import { useEffect, useState } from "react";
import { KeyRound, Monitor, Moon, Palette as PaletteIcon, Settings2, Sun } from "lucide-react";

import { useAutoRefresh } from "@/hooks/useAutoRefresh";
import { useFonts } from "@/hooks/useFonts";
import { usePalette } from "@/hooks/usePalette";
import { useTheme, type Theme } from "@/hooks/useTheme";
import {
  DEFAULT_FONT_LEADS,
  previewFontFamily,
  sanitizeFontList,
  type FontPreferences,
  type FontSlot,
} from "@/lib/fonts";
import { PALETTE_META, type Palette } from "@/lib/palette";
import { Radio, RadioGroup } from "@heroui/react";
import { Modal } from "@/components/ui/Modal";
import { SshKeyManager } from "@/components/SshKeyManager";
import { Button } from "@/components/ui/Button";
import { Checkbox } from "@/components/ui/Checkbox";
import { Input } from "@/components/ui/Input";
import { cn } from "@/lib/utils";

type SettingsSection = "general" | "appearance" | "ssh";

const SECTIONS: { id: SettingsSection; label: string; icon: typeof PaletteIcon }[] = [
  { id: "general", label: "General", icon: Settings2 },
  { id: "appearance", label: "Appearance", icon: PaletteIcon },
  { id: "ssh", label: "SSH Keys", icon: KeyRound },
];

interface SettingsModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function SettingsModal({ open, onOpenChange }: SettingsModalProps): React.JSX.Element {
  const [section, setSection] = useState<SettingsSection>("general");

  // Reopening always lands on the first section instead of resuming the
  // previous one.
  useEffect(() => {
    if (open) setSection("general");
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
                "h-auto w-full justify-start flex items-center gap-2 rounded-md px-3 py-2 text-sm text-left transition-colors duration-base",
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
          {section === "general" ? <GeneralSection /> : null}
          {section === "appearance" ? <AppearanceSection /> : null}
          {section === "ssh" ? <SshKeyManager /> : null}
        </div>
      </div>
    </Modal>
  );
}

// ─── General ──────────────────────────────────────────────────────────────

function GeneralSection(): React.JSX.Element {
  const { autoRefresh, setAutoRefresh } = useAutoRefresh();
  return (
    <div className="flex flex-col gap-5">
      <section className="flex flex-col gap-2">
        <h3 className="text-xs font-medium uppercase tracking-wide text-text-muted">
          Auto refresh
        </h3>
        <Checkbox checked={autoRefresh} onChange={setAutoRefresh} className="text-sm">
          Refresh repository data every minute
        </Checkbox>
        <p className="text-xs text-text-muted">
          Refreshes commits, branches and panels, and fetches from the remote. Never pulls or
          pushes.
        </p>
      </section>
    </div>
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
  const { fonts, saveFonts } = useFonts();
  // Draft edits stay local; nothing is applied until Save.
  const [draft, setDraft] = useState<FontPreferences>(fonts);
  const [justSaved, setJustSaved] = useState(false);

  useEffect(() => {
    if (!justSaved) return;
    const timer = window.setTimeout(() => setJustSaved(false), 2000);
    return () => window.clearTimeout(timer);
  }, [justSaved]);

  const dirty =
    sanitizeFontList(draft.sans) !== fonts.sans || sanitizeFontList(draft.mono) !== fonts.mono;

  const setDraftSlot = (slot: FontSlot, value: string): void => {
    setDraft((prev) => ({ ...prev, [slot]: value }));
    setJustSaved(false);
  };

  const handleSave = (): void => {
    setDraft(saveFonts(draft)); // realign with sanitized values
    setJustSaved(true);
  };

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

      <section className="flex flex-col gap-2">
        <h3 className="text-xs font-medium uppercase tracking-wide text-text-muted">Fonts</h3>
        <FontField
          slot="sans"
          label="UI font"
          hint="Interface text. Leads the platform fallback chain; CJK fallbacks stay active."
          value={draft.sans}
          onChange={(value) => setDraftSlot("sans", value)}
        />
        <FontField
          slot="mono"
          label="Mono font"
          hint="Diff, blame, graph, commit message and other code surfaces."
          value={draft.mono}
          onChange={(value) => setDraftSlot("mono", value)}
        />
        <div className="flex items-center gap-3">
          <Button variant="primary" size="sm" disabled={!dirty} onClick={handleSave}>
            Save
          </Button>
          <span aria-live="polite" className="text-xs text-text-muted">
            {justSaved ? "Saved — applied app-wide" : null}
          </span>
        </div>
      </section>
    </div>
  );
}

const FONT_PREVIEW_SAMPLE = "Aa Bb 0123 — The quick brown fox · 中文字体预览 0O1lI";

function FontField({
  slot,
  label,
  hint,
  value,
  onChange,
}: {
  slot: FontSlot;
  label: string;
  hint: string;
  value: string;
  onChange: (value: string) => void;
}): React.JSX.Element {
  const sanitized = sanitizeFontList(value);
  return (
    <div className="flex flex-col gap-1.5">
      <Input
        label={label}
        description={hint}
        placeholder={`${DEFAULT_FONT_LEADS[slot]} (default)`}
        value={value}
        onChange={onChange}
      />
      <div className="flex items-center justify-between gap-2">
        <p
          className="min-w-0 flex-1 truncate text-sm text-text-secondary"
          style={{ fontFamily: previewFontFamily(sanitized, slot) }}
        >
          {FONT_PREVIEW_SAMPLE}
        </p>
        {value === "" ? null : (
          <Button variant="ghost" size="sm" onClick={() => onChange("")}>
            Reset
          </Button>
        )}
      </div>
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
