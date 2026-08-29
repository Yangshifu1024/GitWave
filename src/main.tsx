import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
// Bundled fallback faces: UI text uses platform-native fonts (SF Pro /
// Segoe UI); Roboto Mono is bundled because it leads --font-mono and must
// render identically on every platform.
import "@fontsource/roboto-mono/400.css";
import "@fontsource/roboto-mono/500.css";
import { applyInitialFonts } from "./lib/fonts";
import { applyInitialPalette } from "./lib/palette";
import App from "./App";
import "./styles/tokens.css";

const queryClient = new QueryClient();

// Suppress the WebView's default context menu everywhere except editable
// elements (keep native copy/paste there). App ContextMenus attach their
// own handlers and are unaffected.
window.addEventListener("contextmenu", (e) => {
  const target = e.target as HTMLElement | null;
  if (target?.closest("input, textarea, [contenteditable='true']")) return;
  e.preventDefault();
});

// Apply theme + palette + font preferences before React mounts to avoid FOUC.
applyInitialPreferences();

const rootEl = document.getElementById("root");
if (!rootEl) {
  throw new Error("Root element #root not found in index.html");
}

createRoot(rootEl).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </StrictMode>,
);

function applyInitialPreferences(): void {
  const root = document.documentElement;
  const storedTheme = localStorage.getItem("gitwave-theme");
  if (storedTheme === "dark") {
    root.classList.add("dark");
    root.dataset.theme = "dark";
  } else if (storedTheme === "light") {
    root.classList.add("light");
    root.dataset.theme = "light";
  } else {
    // Our tokens still follow prefers-color-scheme when no class is set;
    // HeroUI keys off data-theme, so resolve it before first paint.
    root.dataset.theme = window.matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light";
  }
  applyInitialPalette();
  applyInitialFonts();
}
