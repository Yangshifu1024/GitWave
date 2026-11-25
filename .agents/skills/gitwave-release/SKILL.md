---
name: gitwave-release
description: 'Cut a GitWave release — currently dormant. main holds the GPUI Kit rewrite without a release pipeline; the skill documents why there is nothing to release and what still applies until the pipeline exists.'
---

# GitWave release

**There is nothing to release from `main` yet.** `main` holds the GPUI Kit rewrite, which is still being built (ADR 0007, `docs/tech/decisions/00-overview.md`). Its release pipeline — packaging, signing, notarization and signed updates — is milestone M7 of `docs/tasks/refactor-gpui-kit-desktop/plan.md`. Until that milestone lands, do not tag or cut a release from `main`: pushing a tag starts no build.

The last released version is **v0.11.0**, a Tauri application. Its source lives at the `v0.11.0` tag and at commit `8466d55`, the last one before it was removed from `main`. The installer stays available from the releases page.

## What still applies

- **The version** is `[workspace.package] version` in `Cargo.toml`, which every crate in the workspace inherits. `scripts/bump-version.sh <x.y.z>` rewrites it and refreshes `Cargo.lock`. It stays `0.11.0` until the first release of the new application is decided.
- **README and the site** still present v0.11.0 as the released version; README also says that `main` is the rewrite in progress. Both stay that way until the same decision.
- **A release is still a human decision.** Nothing in this repository commits, pushes or tags on its own.

When M7 lands, rewrite this skill around the new pipeline instead of extending this page.
