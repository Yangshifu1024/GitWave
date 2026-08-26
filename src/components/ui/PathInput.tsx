// PathInput — Input + Browse button that opens a native file/directory
// picker via @tauri-apps/plugin-dialog. Replaces manual path typing
// throughout the UI (init repo / clone dest / add local / relink / ssh key).

import type { KeyboardEvent } from "react";
import { open as openDialog } from "@tauri-apps/plugin-dialog";
import { File, FolderOpen } from "lucide-react";

import { Button } from "./Button";
import { Input } from "./Input";

export interface PathInputProps {
  value: string;
  onChange: (v: string) => void;
  /** When true, opens a directory picker; otherwise a file picker. */
  directory?: boolean;
  /** File-extension filters for the picker (ignored when `directory` is true). */
  filters?: Array<{ name: string; extensions: string[] }>;
  placeholder?: string;
  /** Inline error text rendered beneath the input. `null` is allowed
   *  for symmetry with callers that store `string | null`. */
  error?: string | null;
  disabled?: boolean;
  autoFocus?: boolean;
  onKeyDown?: (e: KeyboardEvent<HTMLInputElement>) => void;
  /** Forwarded to the underlying input — useful for `id`, `aria-*`, etc. */
  id?: string;
}

export function PathInput({
  value,
  onChange,
  directory = false,
  filters,
  placeholder,
  error,
  disabled,
}: PathInputProps): React.JSX.Element {
  async function browse(): Promise<void> {
    try {
      const result = await openDialog({
        directory,
        multiple: false,
        filters,
      });
      if (typeof result === "string" && result.length > 0) {
        onChange(result);
      }
    } catch (e) {
      // User cancelled or dialog error — silent fail (do not show toast here;
      // higher-level callers handle errors).
      console.error("PathInput: dialog open failed", e);
    }
  }

  return (
    <div className="flex gap-2">
      <Input
        value={value}
        onChange={onChange}
        placeholder={placeholder}
        error={error}
        disabled={disabled}
        className="flex-1 min-w-0"
      />
      <Button
        type="button"
        variant="secondary"
        size="md"
        onClick={() => void browse()}
        disabled={disabled}
        className="shrink-0"
        aria-label={directory ? "Browse for directory" : "Browse for file"}
      >
        {directory ? <FolderOpen size={14} /> : <File size={14} />}
        <span>Browse…</span>
      </Button>
    </div>
  );
}
