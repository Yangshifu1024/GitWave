# Workbench navigation and title bar actions

- Use the workbench navigator for History and Changes. Keep the file count in the sidebar and the mode switcher in the workbench header when the sidebar is collapsed.
- Translate the shared mode labels and the view-commit action in Simplified Chinese.
- Keep Stash separate and group Fetch, Pull and Push. Render each action as a button with its existing handler and enablement rules. Keep hover hints on the external tool shortcuts.

## Verification

Switch languages with the sidebar expanded and collapsed. At the minimum window width and a wider size, check hit targets after hover, keyboard activation, counters, disabled states and the title bar drag region. Run `make check`.

Chromium 156 exercised the real navigation and title bar with simulated Tauri IPC. At 900, 1024 and 1440 px, all 24 center and edge clicks after a 390 ms hover activated the intended button once. Keyboard activation, both languages and sidebar layouts, counters and disabled states passed. The empty title bar hit the drag surface at all three widths. Native window movement and live Git remotes were outside this browser check.
