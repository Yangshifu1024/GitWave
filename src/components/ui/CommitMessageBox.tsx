import { type ChangeEvent, type KeyboardEvent, useRef } from "react";
import { Sparkles } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";

export interface CommitMessageBoxProps {
  value: string;
  onChange: (value: string) => void;
  onSubmit: () => void;
  onAiGenerate?: () => void;
  amendMessage?: string | null;
  disabled?: boolean;
  className?: string;
}

/**
 * Multi-line commit message textarea with AI generate button
 * and optional Amend prefill.
 */
export function CommitMessageBox({
  value,
  onChange,
  onSubmit,
  onAiGenerate,
  amendMessage = null,
  disabled = false,
  className,
}: CommitMessageBoxProps): React.JSX.Element {
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>): void => {
    // Cmd/Ctrl + Enter submits
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      if (!disabled && value.trim()) {
        onSubmit();
      }
    }
  };

  const canSubmit = value.trim().length > 0 && !disabled;

  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <textarea
        ref={textareaRef}
        value={value}
        onChange={(e: ChangeEvent<HTMLTextAreaElement>) =>
          onChange(e.currentTarget.value)
        }
        onKeyDown={handleKeyDown}
        placeholder="feat: your commit message here…"
        disabled={disabled}
        rows={4}
        maxLength={500}
        className={cn(
          "flex-1 w-full resize-none",
          "rounded-md border border-border-default bg-bg-elevated",
          "px-3 py-2 text-sm text-text-primary placeholder:text-text-muted",
          "transition-colors duration-200",
          "focus:outline-none focus:ring-2 focus:ring-accent focus:ring-offset-2",
          "disabled:opacity-50 disabled:cursor-not-allowed",
          amendMessage != null && "border-warning/50",
        )}
        aria-label="Commit message"
      />

      {/* Action buttons */}
      <div className="flex items-center gap-2">
        <Button
          variant="primary"
          size="sm"
          disabled={!canSubmit}
          onClick={onSubmit}
          className="flex-1"
        >
          {amendMessage != null ? "Amend commit" : "Commit"}
        </Button>

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

        {onAiGenerate && (
          <Button
            variant="ghost"
            size="sm"
            disabled={disabled}
            onClick={onAiGenerate}
            title="Generate commit message with AI"
            aria-label="Generate commit message with AI"
          >
            <Sparkles size={16} />
          </Button>
        )}
      </div>

      {/* Character count hint */}
      <p className="text-xs text-text-muted text-right">
        {value.length > 0 && value.length < 10
          ? `${value.length} chars`
          : value.length >= 72
            ? `${value.length} chars (first line > 72)`
            : null}
      </p>
    </div>
  );
}
