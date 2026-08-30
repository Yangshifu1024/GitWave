import { type KeyboardEvent, useRef } from "react";
import { InputGroup, TextField } from "@heroui/react";
import { Loader2, Sparkles } from "lucide-react";
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
  disabled?: boolean;
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
  disabled = false,
  className,
}: CommitMessageBoxProps): React.JSX.Element {
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>): void => {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      if (!disabled && value.trim()) {
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

  const canSubmit = value.trim().length > 0 && !disabled;
  // Git convention caps the subject line (~50 soft / 72 hard); the body may
  // be any length, so the warning must look at the first line only.
  const firstLineLength = (value.split("\n", 1)[0] ?? "").length;

  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <div className="flex flex-wrap gap-1">
        {COMMIT_TYPES.map((type) => (
          <Button
            key={type}
            type="button"
            variant="ghost"
            size="sm"
            disabled={disabled}
            onClick={() => applyType(type)}
            className={cn(
              "h-auto rounded-sm border border-border-subtle px-1.5 py-0.5",
              "font-mono text-[10px] text-text-secondary",
              "hover:border-accent hover:text-accent",
              "disabled:opacity-40 disabled:pointer-events-none",
            )}
          >
            {type}
          </Button>
        ))}
      </div>
      <TextField
        value={value}
        onChange={onChange}
        isDisabled={disabled}
        className="w-full"
        aria-label="Commit message"
      >
        <InputGroup
          fullWidth
          className={cn("rounded-md", amendMessage != null && "border-warning/50")}
        >
          <InputGroup.TextArea
            ref={textareaRef}
            onKeyDown={handleKeyDown}
            placeholder="feat: your commit message here…"
            rows={6}
            maxLength={500}
            className="w-full resize-none px-3 py-2 text-sm"
          />
        </InputGroup>
      </TextField>
      {value.length > 0 && (value.length < 10 || firstLineLength > 72) ? (
        <p className="text-xs text-text-muted text-right -mt-1">
          {value.length < 10
            ? `${value.length} chars`
            : `${value.length} chars (first line > 72: ${firstLineLength})`}
        </p>
      ) : null}
      <div className="flex items-center justify-end gap-2">
        {amendMessage != null && (
          <Button
            variant="secondary"
            size="sm"
            disabled={disabled}
            onClick={() => {
              onChange(amendMessage);
              textareaRef.current?.focus();
            }}
            title="Prefill with last commit message"
          >
            Amend
          </Button>
        )}
        {onAiGenerate ? (
          <Button
            variant="ghost"
            size="sm"
            disabled={disabled || aiLoading}
            onClick={onAiGenerate}
            title="Generate commit message with AI"
            aria-label="Generate commit message with AI"
          >
            {aiLoading ? (
              <Loader2 size={14} className="animate-spin" aria-hidden="true" />
            ) : (
              <Sparkles size={14} />
            )}
            {aiLoading ? "Generating…" : "Generate"}
          </Button>
        ) : null}
        <Button variant="primary" size="sm" disabled={!canSubmit} onClick={onSubmit}>
          {amendMessage != null ? "Amend commit" : "Commit"}
        </Button>
      </div>
    </div>
  );
}
