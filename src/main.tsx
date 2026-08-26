import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import App from "./App";
import "./styles/tokens.css";
import "./App.css";

const queryClient = new QueryClient();

// Apply initial theme before React mounts to avoid FOUC.
applyInitialTheme();

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

function applyInitialTheme(): void {
  const stored = localStorage.getItem("gitwave-theme");
  const root = document.documentElement;
  if (stored === "dark") {
    root.classList.add("dark");
  } else if (stored === "light") {
    root.classList.add("light");
  }
  // else: leave class off; CSS @media (prefers-color-scheme: dark) takes over
}