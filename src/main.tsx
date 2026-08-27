import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "@fontsource/roboto/400.css";
import "@fontsource/roboto/500.css";
import "@fontsource/roboto/700.css";
import "@fontsource/roboto-mono/400.css";
import "@fontsource/roboto-mono/500.css";
import { applyInitialPalette } from "./lib/palette";
import App from "./App";
import "./styles/tokens.css";

const queryClient = new QueryClient();

// Suppress the WebView's default context menu everywhere except editable
// elements (keep native copy/paste there). Radix ContextMenus attach their
// own handlers and are unaffected.
window.addEventListener("contextmenu", (e) => {
  const target = e.target as HTMLElement | null;
  if (target?.closest("input, textarea, [contenteditable='true']")) return;
  e.preventDefault();
});

// Apply theme + palette preferences before React mounts to avoid FOUC.
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
  } else if (storedTheme === "light") {
    root.classList.add("light");
  }
  // else: leave class off; CSS @media (prefers-color-scheme: dark) takes over
  applyInitialPalette();
}
