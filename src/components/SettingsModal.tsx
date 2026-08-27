import { usePalette } from "@/hooks/usePalette";
import { PALETTE_META, type Palette } from "@/lib/palette";
import { Modal } from "@/components/ui/Modal";
import { cn } from "@/lib/utils";

interface SettingsModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function SettingsModal({ open, onOpenChange }: SettingsModalProps): React.JSX.Element {
  const { palette, setPalette } = usePalette();

  return (
    <Modal open={open} onOpenChange={onOpenChange} title="Settings" size="sm">
      <div className="flex flex-col gap-2">
        <div className="text-xs font-medium tracking-wide text-text-muted uppercase">
          Appearance
        </div>
        <div role="radiogroup" aria-label="Color palette" className="grid grid-cols-2 gap-2">
          {(Object.keys(PALETTE_META) as Palette[]).map((id) => (
            <PaletteOption
              key={id}
              meta={PALETTE_META[id]}
              selected={palette === id}
              onSelect={() => setPalette(id)}
            />
          ))}
        </div>
      </div>
    </Modal>
  );
}

function PaletteOption({
  meta,
  selected,
  onSelect,
}: {
  meta: (typeof PALETTE_META)[Palette];
  selected: boolean;
  onSelect: () => void;
}): React.JSX.Element {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      onClick={onSelect}
      className={cn(
        "rounded-lg border p-2.5 text-left transition-colors",
        "focus-visible:outline-2 focus-visible:outline-offset-1",
        selected
          ? "border-accent bg-accent/5 ring-accent/30 ring-1"
          : "border-border-default hover:border-border-strong",
      )}
    >
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
            <span
              key={lane}
              className="h-1 flex-1 rounded-full"
              style={{ backgroundColor: lane }}
            />
          ))}
        </span>
        <span
          className="size-3 shrink-0 rounded-full border border-black/10"
          style={{ backgroundColor: meta.swatch.accent }}
        />
      </span>
      <span className="block text-sm font-medium text-text-primary">{meta.name}</span>
      <span className="mt-0.5 block text-xs leading-snug text-text-secondary">
        {meta.description}
      </span>
    </button>
  );
}
