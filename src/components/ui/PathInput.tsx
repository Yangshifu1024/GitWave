// PathInput — Input + trailing browse control (inside the field's right
// edge) that opens a native file/directory picker via
// @tauri-apps/plugin-dialog. Replaces manual path typing throughout the UI
// (init repo / clone dest / add local / worktree / relink / ssh key).

import type { KeyboardEvent } from "react";
import { open as openDialog } from "@tauri-apps/plugin-dialog";
import { File, FolderOpen } from "lucide-react";

import { Input } from "./Input";

export interface PathInputProps {
  value: string;
  onChange: (v: string) => void;
  /** Allow selecting several entries at once in the picker. Picked paths are
   *  reported through `onPickMany` instead of `onChange` (used by batch add,
   *  where the list is accumulated rather than replacing a single value). */
  multiple?: boolean;
  /** Receives every path from a multi-selection. Only called when `multiple`. */
  onPickMany?: (paths: string[]) => void;
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
  multiple = false,
  onPickMany,
  directory = false,
  filters,
  placeholder,
  error,
  disabled,
  autoFocus,
  onKeyDown,
  id,
}: PathInputProps): React.JSX.Element {
  async function browse(): Promise<void> {
    try {
      const result = await openDialog({
        directory,
        multiple,
        filters,
      });
      if (multiple) {
        const selected: unknown[] = Array.isArray(result) ? result : [];
        const paths: string[] = [];
        for (const entry of selected) {
          if (typeof entry === "string" && entry.length > 0) paths.push(entry);
        }
        if (paths.length > 0) onPickMany?.(paths);
        return;
      }
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
    <Input
      value={value}
      onChange={onChange}
      placeholder={placeholder}
      error={error}
      disabled={disabled}
      autoFocus={autoFocus}
      onKeyDown={onKeyDown}
      id={id}
      className="w-full"
      suffix={
        <button
          type="button"
          onClick={() => void browse()}
          disabled={disabled}
          aria-label={directory ? "Browse for directory" : "Browse for file"}
          title={directory ? "Browse for directory" : "Browse for file"}
          className="flex size-6 items-center justify-center rounded text-text-muted transition-colors hover:bg-bg-secondary hover:text-text-primary disabled:cursor-default disabled:opacity-50"
        >
          {directory ? <FolderOpen size={14} /> : <File size={14} />}
        </button>
      }
    />
  );
}
