# Website screenshots

The getting-started guide shows six screenshots per language, twelve in total. The
guide pages are already written and point at the file names below; until a file
exists the page shows a dashed placeholder box instead of a broken image.

## How to add a screenshot

1. Take the shot with the app UI in the language of the page it belongs to
   (Settings → General → UI language switches between English and 中文).
2. Save it as a PNG with the exact file name below, in the matching folder:
   - English guide (`/getting-started.html`) → `site/shots/en/`
   - Chinese guide (`/getting-started.zh.html`) → `site/shots/zh/`
3. Then replace the placeholder block in the page with the real image:

   ```html
   <figure class="shot">
     <img src="/shots/en/ui-map.png" alt="…" />
     <figcaption>…</figcaption>
   </figure>
   ```

   The `<figure>`, its `<figcaption>` and the surrounding step stay as they are —
   only the `<div class="shot-ph">…</div>` block is swapped for the `<img>`.

## File names

| File name          | Where it goes in the guide      | What the shot should show                                                                 |
| ------------------ | ------------------------------- | ----------------------------------------------------------------------------------------- |
| `ui-map.png`       | Before step 1, the UI map       | The whole window: title bar with the workspace selector, sidebar, commit graph, inspector |
| `add-local-repo.png` | Main path, step 1             | The **Add existing local repos** dialog with at least one repository path already picked  |
| `pick-commit.png`  | Main path, step 2               | The commit graph with a commit selected, so the diff on the right is populated             |
| `commit-changes.png` | Main path, step 3             | The working-copy window with staged files and a commit message typed in                    |
| `push.png`         | Main path, step 4               | The push confirmation showing the target remote and branch                                 |
| `clone.png`        | The clone branch                | The **Clone remote repo** dialog with a URL filled in and the destination path shown       |

## Notes

- Keep the window at a normal size; the guide renders shots at the full width of
  a 760px reading column.
- Avoid showing real remote URLs, tokens, e-mail addresses or file paths that
  belong to a private project.
- `assets/gitwave-screenshot.png` in the repository root is a different thing:
  it is the README image, not a website asset.
