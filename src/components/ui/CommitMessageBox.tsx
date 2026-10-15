import { type KeyboardEvent, useRef } from "react";
import { InputGroup, TextField } from "@heroui/react";
import { Loader2, Sparkles } from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";

export interface CommitMessageBoxProps {
  value: string;
  onChange: (value: string) => void;
  onSubmit: () => void;
  onAiGenerate?: () => void;
  /** True while AI message generation is in flight — the Generate button
   *  shows a spinner and locks. */
  aiLoading?: boolean;
  amendMessage?: string | null;
  /** Present when an Amend entry should be offered (HEAD exists, on a
   *  branch); invoking it asks the parent to confirm the history rewrite.
   *  Parent then sets `amendMessage` to enter amend mode. */
  onAmend?: () => void;
  disabled?: boolean;
  submitDisabled?: boolean;
  aiDisabled?: boolean;
  onRestoreDraft?: () => void;
  className?: string;
}

/** Standard Conventional Commits type headers offered above the message box. */
const COMMIT_TYPES = [
  "feat",
  "fix",
  "chore",
  "docs",
  "style",
  "refactor",
  "perf",
  "test",
  "build",
  "ci",
] as const;

/**
 * Commit message textarea with AI generate (start) and Commit (end) on the row below.
 */
export function CommitMessageBox({
  value,
  onChange,
  onSubmit,
  onAiGenerate,
  aiLoading = false,
  amendMessage = null,
  onAmend,
  disabled = false,
  submitDisabled = false,
  aiDisabled = false,
  onRestoreDraft,
  className,
}: CommitMessageBoxProps): React.JSX.Element {
  const { t } = useTranslation();
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>): void => {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      if (!disabled && !submitDisabled && value.trim()) {
        onSubmit();
      }
    }
  };

  const applyType = (type: string): void => {
    // Replace an existing conventional prefix instead of stacking another.
    const stripped = value.replace(/^[a-z]+(\([^)]*\))?:\s*/i, "");
    const next = `${type}: ${stripped}`;
    const cursor = type.length + 2; // right after "type: "
    onChange(next);
    // The controlled value updates on the next render — place the caret then.
    requestAnimationFrame(() => {
      textareaRef.current?.focus();
      textareaRef.current?.setSelectionRange(cursor, cursor);
    });
  };

  const canSubmit = value.trim().length > 0 && !disabled && !submitDisabled;
  // Git convention caps the subject line (~50 soft / 72 hard); the body may
  // be any length, so the warning must look at the first line only.
  const firstLineLength = (value.split("\n", 1)[0] ?? "").length;

  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <select
        aria-label={t("app.workbench.commitType")}
        value=""
        disabled={disabled}
        className="h-7 w-fit max-w-full rounded-md border border-border-subtle bg-bg-panel px-2 text-xs text-text-secondary"
        onChange={(event) => {
          if (event.target.value) applyType(event.target.value);
        }}
      >
        <option value="">{t("app.workbench.commitType")}</option>
        {COMMIT_TYPES.map((type) => (
          <option key={type} value={type}>
            {type}
          </option>
        ))}
      </select>
      <TextField
        value={value}
        onChange={onChange}
        isDisabled={disabled}
        className="w-full"
        aria-label={t("changes.messageBox.ariaLabel")}
      >
        <InputGroup
          fullWidth
          className={cn("rounded-md", amendMessage != null && "border-warning/50")}
        >
          <InputGroup.TextArea
            ref={textareaRef}
            onKeyDown={handleKeyDown}
            placeholder={t("changes.messageBox.placeholder")}
            rows={4}
            className="w-full resize-none px-3 py-2 text-sm"
          />
        </InputGroup>
      </TextField>
      {value.length > 0 && (value.length < 10 || firstLineLength > 72) ? (
        <p className="text-xs text-text-muted text-right -mt-1">
          {value.length < 10
            ? t("changes.messageBox.charCount", { chars: value.length })
            : t("changes.messageBox.charCountFirstLine", {
                chars: value.length,
                firstLine: firstLineLength,
              })}
        </p>
      ) : null}
      <div className="flex flex-wrap items-center justify-end gap-2">
        {onRestoreDraft ? (
          <Button variant="ghost" size="sm" disabled={disabled} onClick={onRestoreDraft}>
            {t("changes.messageBox.restoreDraft")}
          </Button>
        ) : null}
        {onAmend && amendMessage == null && (
          <Button
            variant="secondary"
            size="sm"
            disabled={disabled}
            onClick={onAmend}
            title={t("changes.amend.buttonTitle")}
          >
            {t("changes.amend.button")}
          </Button>
        )}
        {amendMessage != null && (
          <Button
            variant="secondary"
            size="sm"
            disabled={disabled}
            onClick={() => {
              onChange(amendMessage);
              textareaRef.current?.focus();
            }}
            title={t("changes.messageBox.amendTitle")}
          >
            {t("changes.messageBox.amend")}
          </Button>
        )}
        {onAiGenerate ? (
          <Button
            variant="ghost"
            size="sm"
            disabled={disabled || aiLoading || aiDisabled}
            onClick={onAiGenerate}
            title={t("changes.messageBox.generateTitle")}
            aria-label={t("changes.messageBox.generateTitle")}
          >
            {aiLoading ? (
              <Loader2 size={14} className="animate-spin" aria-hidden="true" />
            ) : (
              <Sparkles size={14} />
            )}
            {aiLoading ? t("changes.messageBox.generating") : t("changes.messageBox.generate")}
          </Button>
        ) : null}
        <Button variant="primary" size="sm" disabled={!canSubmit} onClick={onSubmit}>
          {amendMessage != null
            ? t("changes.messageBox.amendCommit")
            : t("changes.messageBox.commit")}
        </Button>
      </div>
    </div>
  );
}
