import { useEffect, useState } from "react";
import { KeyRound, Monitor, Moon, Palette as PaletteIcon, Settings2, Sun } from "lucide-react";
import { useTranslation } from "react-i18next";

import { useAutoRefresh } from "@/hooks/useAutoRefresh";
import { useAutoUpdateSetting, useCheckForUpdates } from "@/hooks/useUpdater";
import { useFonts } from "@/hooks/useFonts";
import { usePalette } from "@/hooks/usePalette";
import { useTheme, type Theme } from "@/hooks/useTheme";
import { getAppVersion } from "@/lib/api";
import { useUpdaterStore } from "@/stores/updaterStore";
import {
  DEFAULT_FONT_LEADS,
  previewFontFamily,
  sanitizeFontList,
  type FontPreferences,
  type FontSlot,
} from "@/lib/fonts";
import { PALETTE_META, type Palette } from "@/lib/palette";
import { Radio, RadioGroup } from "@heroui/react";
import {
  AI_LANGUAGES,
  AI_LANGUAGE_NATIVE_NAMES,
  readStoredAiLanguage,
  setAiLanguage,
  type AiLanguage,
} from "@/i18n/aiLanguage";
import { changeUiLanguage, UI_LANGUAGES, type UiLanguage } from "@/i18n";
import { Select } from "@/components/ui/Select";
import { Modal } from "@/components/ui/Modal";
import { SshKeyManager } from "@/components/SshKeyManager";
import { Button } from "@/components/ui/Button";
import { Checkbox } from "@/components/ui/Checkbox";
import { Input } from "@/components/ui/Input";
import { cn } from "@/lib/utils";

type SettingsSection = "general" | "appearance" | "ssh";

const SECTION_KEY: Record<SettingsSection, string> = {
  general: "settings.sections.general",
  appearance: "settings.sections.appearance",
  ssh: "settings.sections.ssh",
};

const SECTIONS: { id: SettingsSection; icon: typeof PaletteIcon }[] = [
  { id: "general", icon: Settings2 },
  { id: "appearance", icon: PaletteIcon },
  { id: "ssh", icon: KeyRound },
];

const UI_LANGUAGE_LABEL: Record<UiLanguage, string> = {
  en: "English",
  "zh-CN": "中文",
};

interface SettingsModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function SettingsModal({ open, onOpenChange }: SettingsModalProps): React.JSX.Element {
  const { t } = useTranslation();
  const [section, setSection] = useState<SettingsSection>("general");

  // Reopening always lands on the first section instead of resuming the
  // previous one.
  useEffect(() => {
    if (open) setSection("general");
  }, [open]);

  return (
    <Modal open={open} onOpenChange={onOpenChange} title={t("settings.title")} size="lg">
      <div className="flex h-[60vh] min-h-0">
        <nav
          aria-label={t("settings.title")}
          className="flex w-44 shrink-0 flex-col gap-0.5 border-r border-border-subtle pr-2"
        >
          {SECTIONS.map(({ id, icon: Icon }) => (
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
              {t(SECTION_KEY[id])}
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
  const { t } = useTranslation();
  const { autoRefresh, setAutoRefresh } = useAutoRefresh();
  return (
    <div className="flex flex-col gap-5">
      <LanguageSection />
      <section className="flex flex-col gap-2">
        <h3 className="text-xs font-medium uppercase tracking-wide text-text-muted">
          {t("settings.general.autoRefresh")}
        </h3>
        <Checkbox checked={autoRefresh} onChange={setAutoRefresh} className="text-sm">
          {t("settings.general.autoRefreshCheckbox")}
        </Checkbox>
        <p className="text-xs text-text-muted">{t("settings.general.autoRefreshHint")}</p>
      </section>
      <UpdatesSection />
    </div>
  );
}

function LanguageSection(): React.JSX.Element {
  const { t, i18n } = useTranslation();
  const [aiLanguage, setAiLanguageState] = useState<AiLanguage>(readStoredAiLanguage);
  const currentUi = (UI_LANGUAGES as readonly string[]).includes(i18n.language)
    ? (i18n.language as UiLanguage)
    : "en";

  return (
    <section className="flex flex-col gap-2">
      <div className="flex flex-col gap-1">
        <Select
          aria-label={t("settings.general.uiLanguage")}
          className="w-full"
          value={currentUi}
          onChange={(value) => changeUiLanguage(value as UiLanguage)}
          options={UI_LANGUAGES.map((lang) => ({ value: lang, label: UI_LANGUAGE_LABEL[lang] }))}
        />
        <p className="text-xs text-text-muted">{t("settings.general.uiLanguageHint")}</p>
      </div>
      <div className="flex flex-col gap-1">
        <Select
          aria-label={t("settings.general.aiLanguage")}
          className="w-full"
          value={aiLanguage}
          onChange={(value) => {
            const next = value as AiLanguage;
            setAiLanguage(next);
            setAiLanguageState(next);
          }}
          options={AI_LANGUAGES.map((lang) => ({
            value: lang,
            label: AI_LANGUAGE_NATIVE_NAMES[lang],
          }))}
        />
        <p className="text-xs text-text-muted">{t("settings.general.aiLanguageHint")}</p>
      </div>
    </section>
  );
}

function UpdatesSection(): React.JSX.Element {
  const { t } = useTranslation();
  const { autoUpdate, setAutoUpdate } = useAutoUpdateSetting();
  const { check, busy } = useCheckForUpdates();
  const phase = useUpdaterStore((s) => s.phase);
  const newVersion = useUpdaterStore((s) => s.newVersion);
  const error = useUpdaterStore((s) => s.error);
  const setModalOpen = useUpdaterStore((s) => s.setModalOpen);
  const [version, setVersion] = useState<string | null>(null);

  useEffect(() => {
    getAppVersion()
      .then(setVersion)
      .catch(() => setVersion(null));
  }, []);

  const found =
    phase === "available" ||
    phase === "manual-download" ||
    phase === "downloading" ||
    phase === "ready";
  const statusText = busy
    ? t("settings.general.checking")
    : phase === "up-to-date"
      ? t("settings.general.upToDate")
      : found
        ? t("settings.general.available", { version: `v${newVersion ?? "?"}` })
        : phase === "error"
          ? (error ?? t("settings.general.checkFailed"))
          : null;

  return (
    <section className="flex flex-col gap-2">
      <h3 className="text-xs font-medium uppercase tracking-wide text-text-muted">
        {t("settings.general.updates")}
      </h3>
      <Checkbox checked={autoUpdate} onChange={setAutoUpdate} className="text-sm">
        {t("settings.general.autoUpdateCheckbox")}
      </Checkbox>
      <p className="text-xs text-text-muted">{t("settings.general.updatesHint")}</p>
      <div className="flex items-center gap-3">
        <Button variant="secondary" size="sm" disabled={busy} onClick={() => void check()}>
          {t("settings.general.checkForUpdates")}
        </Button>
        {found ? (
          <Button variant="secondary" size="sm" onClick={() => setModalOpen(true)}>
            {t("settings.general.viewUpdate")}
          </Button>
        ) : null}
        <span aria-live="polite" className="text-xs text-text-muted">
          {version ? `v${version}` : null}
          {statusText ? ` · ${statusText}` : ""}
        </span>
      </div>
    </section>
  );
}

// ─── Appearance ───────────────────────────────────────────────────────────

const THEME_OPTIONS: { value: Theme; labelKey: string; icon: typeof Sun }[] = [
  { value: "light", labelKey: "settings.appearance.themeLight", icon: Sun },
  { value: "dark", labelKey: "settings.appearance.themeDark", icon: Moon },
  { value: "system", labelKey: "settings.appearance.themeSystem", icon: Monitor },
];

function AppearanceSection(): React.JSX.Element {
  const { t } = useTranslation();
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
        <h3 className="text-xs font-medium uppercase tracking-wide text-text-muted">
          {t("settings.appearance.theme")}
        </h3>
        <RadioGroup
          value={theme}
          onChange={(value) => setTheme(value as Theme)}
          aria-label={t("settings.appearance.theme")}
          className="grid grid-cols-3 gap-2 [&_[data-slot=radio]]:mt-0"
          style={{ display: "grid" }}
        >
          {THEME_OPTIONS.map(({ value, labelKey, icon: Icon }) => (
            <CardOption key={value} value={value} selected={theme === value} label={t(labelKey)}>
              <Icon size={18} className={theme === value ? "text-accent" : "text-text-secondary"} />
            </CardOption>
          ))}
        </RadioGroup>
      </section>

      <section className="flex flex-col gap-2">
        <h3 className="text-xs font-medium uppercase tracking-wide text-text-muted">
          {t("settings.appearance.colorPalette")}
        </h3>
        <RadioGroup
          value={palette}
          onChange={(value) => setPalette(value as Palette)}
          aria-label={t("settings.appearance.colorPalette")}
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
        <h3 className="text-xs font-medium uppercase tracking-wide text-text-muted">
          {t("settings.appearance.fonts")}
        </h3>
        <FontField
          slot="sans"
          label={t("settings.appearance.uiFont")}
          hint={t("settings.appearance.uiFontHint")}
          value={draft.sans}
          onChange={(value) => setDraftSlot("sans", value)}
        />
        <FontField
          slot="mono"
          label={t("settings.appearance.monoFont")}
          hint={t("settings.appearance.monoFontHint")}
          value={draft.mono}
          onChange={(value) => setDraftSlot("mono", value)}
        />
        <div className="flex items-center gap-3">
          <Button variant="primary" size="sm" disabled={!dirty} onClick={handleSave}>
            {t("common.save")}
          </Button>
          <span aria-live="polite" className="text-xs text-text-muted">
            {justSaved ? t("settings.appearance.saved") : null}
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
  const { t } = useTranslation();
  const sanitized = sanitizeFontList(value);
  return (
    <div className="flex flex-col gap-1.5">
      <Input
        label={label}
        description={hint}
        placeholder={`${DEFAULT_FONT_LEADS[slot]} (${t("common.default")})`}
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
            {t("common.reset")}
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
