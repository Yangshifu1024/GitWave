---
name: gitwave-release
description: 'Cut a GitWave release — verify the full gate, bump the version everywhere with the bump script, sync README and the site, commit, and after explicit confirmation push main and the v* tag, then dispatch the Go desktop draft release workflow. Use whenever the user wants to release, ship, publish, or tag a GitWave version, bump the version, or sync the version to README / site — including 发版/发布/出新版本/升个版本/打个 tag/更新 README 和 site — even a bare "release 0.7.15" or "发个版". Even when the user only mentions one piece (e.g. just bump the version), proactively check the other sync points: a half-synced version breaks the release chain.'
---

# GitWave release

Prepare the version and sync README and the site, then commit on `main` and push the commit and version tag after confirmation. Dispatch **Go desktop release** on the matching `main` commit. The one-time exporter uses the preserved source checkpoint from the migration plan. CI produces a draft for manual review. The [migration plan](../../../docs/tasks/refactor-nsgo-native-ui/plan.md#三端包装更新与回退) records the Go signing inputs and the two fixed transition versions.

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
3. Run `make check` for the Go format, vet, and test gates.

On failure: stop, show the failing output, and let the user decide what to fix. Do not bump.

## 3. Set the version

Run `make bump VERSION=X.Y.Z` and check the result with `make version`. The script updates `cmd/gitwave/nsgo.yaml`. Use the same version in the tag and **Go desktop release** workflow input. Go draft creation checks both against the selected source commit.

Before syncing documentation, check that the version change is confined to `cmd/gitwave/nsgo.yaml`.

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
- **Branding assets** — when app branding changes, sync the corresponding assets under `site/` from the approved artwork. App assets are in `cmd/gitwave/assets/`.
- Residue check: `rg -n -F '<old-version>' README.md site/ cmd/gitwave/nsgo.yaml` and review the version, status and download references. Keep historical release notes.

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

- **Go desktop release** runs from `main` with `version`, `exporter_version` and `bridge_version`. Enable `draft_release` when its tag already points to that commit. Wait for **Create and verify a draft for the verified tag** to compare the uploaded files with the assembled release. Review the three platform packages, `gitwave-update.json`, `latest.json` and `gitwave-legacy-transition.json`. On later Go releases, use the same exporter and bridge versions as the previous published Go release.

Wait for CI and verify the draft assets before the user publishes. Legacy update URLs use the fixed `releases/download/<tag>/<file>` paths. The Go manifest binds each artifact hash and the installed Windows EXE hash. If a check fails, keep the release in draft.
