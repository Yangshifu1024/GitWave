---
name: gitwave-release
description: Cut a GitWave release — verify the full gate, bump the version everywhere with the bump script, sync README and the site, commit, and after explicit confirmation push main and the v* tag that triggers the three-platform release CI (draft release + updater manifest). Use whenever the user wants to release, ship, publish, or tag a GitWave version, bump the version, or sync the version to README / site — including 发版/发布/出新版本/升个版本/打个 tag/更新 README 和 site — even a bare "release 0.7.15" or "发个版". Even when the user only mentions one piece (e.g. just bump the version), proactively check the other sync points: a half-synced version breaks the release chain.
---

# GitWave release

A release is: bump the version everywhere → sync README + site → commit on `main` → push → push a `v*` tag. The tag push triggers `.github/workflows/build.yml`, which builds Windows NSIS, macOS dmg, and Linux AppImage into a **draft** GitHub Release and generates `latest.json` — the in-app updater manifest — at build time. Publishing the draft is a separate, manual step gated by the user.

**This skill always stops before pushing.** The tag push triggers builds and produces artifacts users install and update from. Follow AGENTS.md: invoking this skill is the user's explicit request to bump / sync / commit, but never push `main` or a tag without the user's explicit yes — not even with every check green.

## 1. Determine the version

- An explicit version in the invocation wins (accept `0.7.15` or `v0.7.15`).
- Otherwise derive a suggestion:
  - Latest tag: `git fetch --tags && git describe --tags --abbrev=0`
  - What's shipping: `git log <latest-tag>..HEAD --oneline`
  - Suggest the next semver from those commits: `feat` → minor, `fix`/`chore`/`docs` → patch; bump the **minor** for anything user-visible while the project is 0.x.
- State the suggestion and the commits it's based on, then ask the user to confirm or override. Never bump an unconfirmed version.
- Never reuse a version that already has a tag: CI's prepare-release job asserts the tag name equals `package.json`'s version, and published releases / updater manifests are immutable. If a release went wrong, pick a new number — don't move the tag (`git tag -d vX.Y.Z` + `git push origin :refs/tags/vX.Y.Z` re-triggers, but only before the draft is published).

## 2. Pre-flight — abort on any failure

Run these before touching any file; a red tree never gets bumped.

1. `git status --porcelain` — must be empty. The bump rewrites 4 files; committing on a dirty tree mixes unrelated changes into the release commit.
2. `git branch --show-current` must be `main`, then `git pull --ff-only` — releases are always cut from an up-to-date main (GitHub Flow).
3. Full gate, CI-equivalent: `make check` (prettier + `cargo fmt` format gates, eslint + clippy + typecheck, frontend and Rust test suites).

On failure: stop, show the failing output, and let the user decide what to fix. Do not bump.

## 3. Bump

```
pnpm bump <version>
```

The script (`scripts/bump-version.mjs`) strips a leading `v` itself. It rewrites `package.json`, `src-tauri/tauri.conf.json`, and `src-tauri/Cargo.toml`, then refreshes the `gitwave` entry in `src-tauri/Cargo.lock` via `cargo update -p gitwave`. `pnpm-lock.yaml` is deliberately untouched — it does not record the root package's own version, and CI installs with `--frozen-lockfile`. Never edit versions by hand.

Verify with `git status --porcelain`: expect exactly those 4 files. Anything else in the diff — investigate before committing.

## 4. Sync user-facing surfaces

- **README.md** — the "Cutting a release" section there is the release checklist's source of truth:
  - Header **Status** line: `**Status:** vX.Y.Z — …`, weaving in this version's most user-visible capability.
  - **Features** list: add a bullet for new capabilities; don't re-list existing ones.
  - Download section: reconcile any version / platform mentions.
- **site/index.html** (official site, auto-deploys to GitHub Pages on push to main), both **en and zh-CN** pages:
  - Hero badge: `<div class="badge">vX.Y.Z · <today></div>` — current version + current date (en `YYYY-MM-DD`, zh `YYYY年M月D日`).
  - Download area: `Latest release: vX.Y.Z` (zh「最新版本：vX.Y.Z」).
  - Feature cards: fold small features into an existing card (e.g. the "Batteries included" list) rather than breaking the 6-card grid rhythm.
- Residue check: `grep -rn "<old-version>" README.md site/ package.json src-tauri/tauri.conf.json src-tauri/Cargo.toml` — GitWave's own version should only appear at the new value (matches inside unrelated dependencies don't count).

## 5. Commit

```
git commit -m "chore: bump version to vX.Y.Z"
```

This matches existing release history; no body needed.

## 6. Summarize, then STOP

Show the user, concretely:

- The version and the commits going out since the previous tag
- What release CI will build once the tag lands: a **draft** release with three-platform installers plus `latest.json`
- That publishing the draft activates the updater — existing installs start receiving the new version from that moment

Then ask, and wait: "Push main + tag vX.Y.Z now?" A yes to the summary is consent to push; silence or anything ambiguous is not.

## 7. Push (only after the user's explicit yes)

```
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin main vX.Y.Z
```

Tag timing is a hard constraint: the tag snapshots HEAD at creation, so it must be created only after the bump commit is merged into `main` and is HEAD. Pushing newer commits never moves an existing tag; a mistagged release gets a new number.

Then offer to monitor the run (`gh run list`, `gh run watch <run-id>`). When CI is green, help the user verify the draft under Releases before they publish: assets present for all three platforms, and `latest.json` carries all three keys (`darwin-aarch64` / `linux-x86_64` / `windows-x86_64`) with non-empty signatures. **Publishing is the user's manual click** — that click flips the draft live and pushes `latest.json` to every installed updater.
