# Terminal and editor pickers

[Accepted proposal and platform matrix](../../pm/features/F020-terminal-picker.md)

Keep stable IDs and discovery/launch rules in Rust. Resolve a selected ID again at launch, pass the active repository root to the terminal or editor and retain the automatic terminal fallback for older IPC callers.

Use the same text picker for both tool types. Check the local installation list when the menu opens, and keep a separate successful choice in local storage for each type. Display an empty state when no apps are detected. Keep the full tool name available in the menu at narrow widths.

Verify first use, direct launch, switching, persistence, unavailable tools, failed and duplicate launches. Check directories with spaces and Unicode, Linux aliases, narrow window layout, localization and the full project checks.

Chromium passed physical clicks, keyboard navigation, language changes and persisted preferences at 900, 1024 and 1440 px using mocked IPC. A Linux X11 smoke test used the catalog and command builder to launch an installed Xfce Terminal. Its shell reported the requested repository path containing spaces, Chinese characters and an ampersand.

The two tool menus size to their contents up to 240 px. Chromium measured 134 × 92 px for the three editor choices before selection and 182 × 92 px with Visual Studio Code marked current. Rows use a 28 px minimum hit target.
