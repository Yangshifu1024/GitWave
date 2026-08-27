export type SelectionModifier = "replace" | "toggle" | "range";

/** Inclusive slice of `ordered` between `anchor` and `target`. */
export function pathsInRange(ordered: string[], anchor: string, target: string): string[] {
  const from = ordered.indexOf(anchor);
  const to = ordered.indexOf(target);
  if (to === -1) return from === -1 ? [] : [anchor];
  if (from === -1) return [target];
  const start = Math.min(from, to);
  const end = Math.max(from, to);
  return ordered.slice(start, end + 1);
}

export function nextFileSelection(
  ordered: string[],
  current: ReadonlySet<string>,
  path: string,
  modifier: SelectionModifier,
  anchor: string | null,
): { selected: Set<string>; anchor: string | null } {
  if (modifier === "toggle") {
    const selected = new Set(current);
    if (selected.has(path)) selected.delete(path);
    else selected.add(path);
    return { selected, anchor };
  }
  if (modifier === "range") {
    return {
      selected: new Set(pathsInRange(ordered, anchor ?? path, path)),
      anchor: anchor ?? path,
    };
  }
  return { selected: new Set([path]), anchor: path };
}

export function modifierFromPointerEvent(event: {
  shiftKey: boolean;
  metaKey: boolean;
  ctrlKey: boolean;
}): SelectionModifier {
  if (event.shiftKey) return "range";
  if (event.metaKey || event.ctrlKey) return "toggle";
  return "replace";
}
