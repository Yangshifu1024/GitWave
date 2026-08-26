import { useCallback, useEffect, useState } from "react";

export type Theme = "light" | "dark" | "system";

export interface UseThemeReturn {
  theme: Theme;
  resolved: "light" | "dark";
  setTheme: (t: Theme) => void;
}

const STORAGE_KEY = "gitwave-theme";

function readStoredTheme(): Theme {
  if (typeof localStorage === "undefined") return "system";
  return (localStorage.getItem(STORAGE_KEY) as Theme) ?? "system";
}

function resolveTheme(stored: Theme): "light" | "dark" {
  if (stored === "system") {
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  }
  return stored;
}

function applyTheme(resolved: "light" | "dark"): void {
  const root = document.documentElement;
  root.classList.remove("light", "dark");
  root.classList.add(resolved);
}

/**
 * Hook for reading and writing the user theme preference.
 * - `theme` is the raw preference ("light" | "dark" | "system")
 * - `resolved` is the actual applied theme after resolving "system" against the OS
 * - `setTheme` persists to localStorage AND updates the <html> class
 * - When theme="system", listens to OS preference changes live
 */
export function useTheme(): UseThemeReturn {
  const [theme, setThemeState] = useState<Theme>(readStoredTheme);
  const [resolved, setResolved] = useState<"light" | "dark">(() => resolveTheme(readStoredTheme()));

  // Apply resolved theme to <html> whenever it changes
  useEffect(() => {
    applyTheme(resolved);
  }, [resolved]);

  // When theme is "system", re-resolve on OS preference changes
  useEffect(() => {
    if (theme !== "system") return;

    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const handler = (): void => {
      const next = resolveTheme("system");
      setResolved(next);
    };
    mq.addEventListener("change", handler);
    return () => mq.removeEventListener("change", handler);
  }, [theme]);

  const setTheme = useCallback((t: Theme) => {
    setThemeState(t);
    localStorage.setItem(STORAGE_KEY, t);
    setResolved(resolveTheme(t));
  }, []);

  return { theme, resolved, setTheme };
}
