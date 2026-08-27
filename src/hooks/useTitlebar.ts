import { invoke } from "@tauri-apps/api/core";
import { useEffect, useState } from "react";

export type TitlebarMode = "custom" | "native" | "pending";

/** Activates tauri-plugin-decoration and reveals the window (starts hidden in tauri.conf). */
export function useTitlebarActivation(): TitlebarMode {
  const [mode, setMode] = useState<TitlebarMode>("pending");

  useEffect(() => {
    void invoke<"custom" | "native">("activate_and_show")
      .then((result) => setMode(result))
      .catch(() => setMode("custom"));
  }, []);

  return mode;
}
