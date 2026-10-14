# Hide the history horizontal scrollbar

## Goal and cause

The history viewport intentionally overflows horizontally to expose graph lanes. Global scrollbar styling gives that overflow a visible 10px bottom scrollbar. Hide that bar without clipping the graph or disabling horizontal navigation.

## Implementation

Scope a `history-scroll` class to the history viewport. Override the global standard scrollbar width so WebKit/Chromium pseudo-element styling can hide only the horizontal scrollbar; retain vertical styling and `overflow-auto`. Update the history design specification.

## Verification

Run `make check` and a production CSS browser fixture checking horizontal bar styling, vertical width, and programmatic scrolling on both axes. Native app trackpad behavior remains a smoke-test item.

Continues PR #181 on `fix/history-column-alignment` per user instruction.

## Results

- `make check`: passed with the local socket/log access required by backend tests.
- Vite production build: passed.
- WebKit fixture using production CSS at 320, 680 and 1440px: zero horizontal scrollbar gutter, vertical scrollbar width 10px, and both `scrollLeft` and `scrollTop` reach 100px.
- Static code review: no blocking findings; see [review](./review.md).
