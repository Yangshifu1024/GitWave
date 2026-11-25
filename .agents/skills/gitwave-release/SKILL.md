---
name: gitwave-release
description: 'Cut a GitWave release — verify the full gate, bump the version everywhere with the bump script, sync README and the site, commit, and after explicit confirmation push main and the v* tag, then dispatch the appropriate draft release workflow. Use whenever the user wants to release, ship, publish, or tag a GitWave version, bump the version, or sync the version to README / site — including 发版/发布/出新版本/升个版本/打个 tag/更新 README 和 site — even a bare "release 0.7.15" or "发个版". Even when the user only mentions one piece (e.g. just bump the version), proactively check the other sync points: a half-synced version breaks the release chain.'
---

# GitWave release

Prepare the version and sync README and the site, then commit on `main` and push the commit and version tag after confirmation. Dispatch **Tauri preference exporter release** on the exporter tag or **Go desktop release** on the matching `main` commit. CI produces a draft for manual review. The [migration plan](../../../docs/tasks/refactor-nsgo-native-ui/plan.md#三端包装更新与回退) records the Go signing inputs and the two fixed transition versions.

**This skill always stops before pushing.** The tag pins the source commit for the subsequent release workflow. Follow AGENTS.md: invoking this skill is the user's explicit request to bump / sync / commit, but never push `main` or a tag without the user's explicit yes — not even with every check green.

## 1. Determine the version

- An explicit version in the invocation wins (accept `0.7.15` or `v0.7.15`).
- Otherwise derive a suggestion:
  - Latest tag: `git fetch --tags && git describe --tags --abbrev=0`
  - What's shipping: `git log <latest-tag>..HEAD --oneline`
  - Suggest the next semver from those commits: `feat` → minor, `fix`/`chore`/`docs` → patch; bump the **minor** for anything user-visible while the project is 0.x.
- State the suggestion and the commits it's based on, then ask the user to confirm or override. Never bump an unconfirmed version.
- Keep release tags fixed to their original commits. When a release needs rebuilt source, choose a new version. Go draft creation checks the remote tag against the candidate commit.

## 2. Pre-flight — abort on any failure

Run these before touching any file; a red tree never gets bumped.

1. `git status --porcelain` — must be empty. A clean tree lets you verify the version script's complete diff.
2. `git branch --show-current` must be `main`, then `git pull --ff-only` — releases are always cut from an up-to-date main (GitHub Flow).
3. Run `make check` for the Go format, vet, and test gates. For a Tauri preference exporter release, also run `make legacy-check`.

On failure: stop, show the failing output, and let the user decide what to fix. Do not bump.

## 3. Set the version

For a Go release, run `make bump VERSION=X.Y.Z` and check the result with `make version`. The script updates `cmd/gitwave/nsgo.yaml`. Use the same version in the tag and **Go desktop release** workflow input. Go draft creation checks both against the selected source commit.

For the Tauri preference exporter, run the version script below.

```
pnpm bump <version>
```

The script (`scripts/bump-version.mjs`) strips a leading `v` itself. It rewrites `package.json`, `src-tauri/tauri.conf.json`, and `src-tauri/Cargo.toml`, then refreshes the `gitwave` entry in `src-tauri/Cargo.lock` via `cargo update -p gitwave`. `pnpm-lock.yaml` is deliberately untouched — it does not record the root package's own version, and CI installs with `--frozen-lockfile`. Never edit versions by hand.

Before syncing documentation, check `git status --porcelain` for the changed version files. Expect `cmd/gitwave/nsgo.yaml` for Go or the four files above for the Tauri exporter.

## 4. Sync user-facing surfaces

- **README.md** — the "Cutting a release" section there is the release checklist's source of truth:
  - Header **Status** line: `**Status:** vX.Y.Z · Release notes · Product scope · Engineering decisions` — update the version number in place; the three links stay as they are.
  - **What's new** list (`### What's new in vX.Y.x`, right below the Status line): add one bullet at the top for this version — `- **vX.Y.Z** — …`, a single line leading with its most user-visible capability — and keep the list to the current and the last few patch releases. Older releases live in the release notes, so trim the tail instead of letting the list grow.
  - **Features** list: add a bullet for new capabilities; don't re-list existing ones.
  - Download section: reconcile any version / platform mentions.
- **site/index.html** (official site — a single English page, auto-deploys to GitHub Pages on push to main):
  - Hero badge: `<div class="badge">vX.Y.Z · <today></div>` — current version + current date (`YYYY-MM-DD`).
  - Download area: `Latest release: vX.Y.Z`.
  - Feature cards: fold small features into an existing card (e.g. the "Diff viewer" or "Batteries included" cards) rather than adding a new one and breaking the grid rhythm.
- **PM docs** — sync the current-version references in `docs/pm/core/README.md`, `01-features.md`, `02-scope.md`, and `03-roadmap.md`; add the release's representative changes to the scope table. Preserve historical version rows and release notes.
- **Branding assets** — when application icons changed since the previous tag, copy `src-tauri/icons/icon.png` to `site/icon.png`, `32x32.png` to `site/favicon-32.png`, and `128x128.png` to `public/app-icon.png`. Keep the frontend favicon in `index.html` pointed at the app icon. Verify these are byte-identical copies; do not regenerate artwork during release sync.
- Residue check: `rg -n -F '<old-version>' README.md site/ cmd/gitwave/nsgo.yaml package.json src-tauri/tauri.conf.json src-tauri/Cargo.toml` and review references in the release's version files, status and download sections. Keep historical release notes and the other build's version.

## 5. Commit

```
git commit -m "chore: bump version to vX.Y.Z"
```

This matches existing release history; no body needed.

## 6. Summarize, then STOP

Show the user, concretely:

- The version and the commits going out since the previous tag
- Which workflow to dispatch, which version it will build, and which installers and update manifests the draft must contain
- That publishing the draft activates the updater — existing installs start receiving the new version from that moment

Then ask, and wait: "Push main + tag vX.Y.Z now?" A yes to the summary is consent to push; silence or anything ambiguous is not.

## 7. Push (only after the user's explicit yes)

```
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin main vX.Y.Z
```

Tag timing is a hard constraint: the tag snapshots HEAD at creation, so it must be created only after the bump commit is merged into `main` and is HEAD. Pushing newer commits never moves an existing tag; a mistagged release gets a new number.

Dispatch the workflow that matches the release:

- **Tauri preference exporter release** uses the `vX.Y.Z` ref. Wait for **Verify preference exporter update feed**, then review the installers and `latest.json` routes for macOS, Windows, and Linux.
- **Go desktop release** runs from `main` with `version`, `exporter_version` and `bridge_version`. Enable `draft_release` when its tag already points to that commit. Wait for **Create and verify a draft for the verified tag** to compare the uploaded files with the assembled release. Review the three platform packages, `gitwave-update.json`, `latest.json` and `gitwave-legacy-transition.json`. On later Go releases, use the same exporter and bridge versions as the previous published Go release.

Wait for CI and verify the draft assets before the user publishes. Legacy update URLs use the fixed `releases/download/<tag>/<file>` paths. The Go manifest binds each artifact hash and the installed Windows EXE hash. If a check fails, keep the release in draft. For a failed Tauri URL rewrite, see `docs/tasks/fix-rewrite-manifest-draft/plan.md`.
