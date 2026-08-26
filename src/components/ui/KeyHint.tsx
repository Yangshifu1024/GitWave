import { cn } from "@/lib/utils";

export interface KeyHintProps {
  /** Single key or array of keys (e.g. ["⌘", "K"] or ["⌘", "⇧", "P"]) */
  keys: string | string[];
  className?: string;
}

/**
 * Renders keyboard shortcut hints as styled pill badges.
 * Used for showing keyboard shortcuts like ⌘K, ⌘⇧P, etc.
 */
export function KeyHint({ keys, className }: KeyHintProps): React.JSX.Element {
  const keyArray = Array.isArray(keys) ? keys : [keys];

  return (
    <span
      className={cn(
        "inline-flex items-center gap-0.5",
        className,
      )}
    >
      {keyArray.map((key, i) => (
        <kbd
          key={i}
          className={cn(
            "inline-flex items-center justify-center",
            "rounded-sm px-1.5 py-0.5",
            "font-mono text-xs font-medium",
            "bg-bg-secondary border border-border-default",
            "text-text-secondary shadow-subtle",
          )}
        >
          {key}
        </kbd>
      ))}
    </span>
  );
}
