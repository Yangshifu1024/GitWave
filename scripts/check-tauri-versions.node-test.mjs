import assert from "node:assert/strict";
import { test } from "node:test";
import { findVersionProblems } from "./check-tauri-versions.mjs";

// Exact resolved versions that broke v0.10.0 on all three platforms.
const lock = `version = 4

[[package]]
name = "tauri"
version = "2.12.1"

[[package]]
name = "tauri-plugin-updater"
version = "2.13.1"

[[package]]
name = "tauri-plugin-os"
version = "2.4.0"

[[package]]
name = "tauri-plugin-opener"
version = "2.7.0"
`;

test("rejects the three plugin mismatches that broke the release", () => {
  const problems = findVersionProblems(
    {
      "@tauri-apps/plugin-updater": "2.12.0",
      "@tauri-apps/plugin-os": "2.3.2",
      "@tauri-apps/plugin-opener": "2.6.0",
    },
    lock,
  );
  assert.equal(problems.length, 3);
  for (const plugin of ["updater", "os", "opener"]) {
    assert.ok(problems.some((problem) => problem.includes(`tauri-plugin-${plugin}`)));
  }
});

test("allows different patches in the same major/minor, including core API", () => {
  assert.deepEqual(
    findVersionProblems(
      {
        "@tauri-apps/api": "2.12.0",
        "@tauri-apps/plugin-updater": "2.13.3",
        "@tauri-apps/plugin-os": "2.4.0",
        "@tauri-apps/plugin-opener": "2.7.1",
      },
      lock.replaceAll("\n", "\r\n"),
    ),
    [],
  );
});

test("rejects major and core minor mismatches", () => {
  assert.equal(findVersionProblems({ "@tauri-apps/api": "3.12.1" }, lock).length, 1);
  assert.equal(findVersionProblems({ "@tauri-apps/api": "2.11.0" }, lock).length, 1);
});

test("fails when an npm plugin has no locked Rust counterpart", () => {
  assert.match(
    findVersionProblems({ "@tauri-apps/plugin-dialog": "2.8.0" }, lock)[0],
    /missing tauri-plugin-dialog/,
  );
});
