#!/usr/bin/env node
//! 统一更新全仓库版本号，并刷新 Cargo.lock。
//!
//! 版本号声明位置：
//!   - package.json              —— pnpm 根包
//!   - src-tauri/tauri.conf.json —— 安装包版本
//!   - src-tauri/Cargo.toml      —— Tauri 应用的 crate 版本
//!   - Cargo.toml                —— 根 workspace 版本，gitwave-core 与 GPUI Kit 应用继承
//! 锁文件：src-tauri/Cargo.lock 的 gitwave 条目由 `cargo update -p gitwave` 刷新；
//! 两个 workspace 里 gitwave-core 的新版本由 cargo metadata 的解析写入；
//! pnpm-lock.yaml 有意不动——它不记录根包自身的版本（CI 用 --frozen-lockfile）。
//!
//! 用法：pnpm bump 0.7.15   （或 node scripts/bump-version.mjs 0.7.15）
import { execSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";

const arg = process.argv[2];
if (!arg || !/^v?\d+\.\d+\.\d+(-[\w.]+)?$/.test(arg)) {
  console.error("用法: pnpm bump <x.y.z>（如 0.7.15）");
  process.exit(1);
}
const version = arg.replace(/^v/, "");

const files = ["package.json", "src-tauri/tauri.conf.json"];

for (const file of files) {
  const s = readFileSync(file, "utf8");
  const next = s.replace(/("version":\s*)"[^"]+"/, `$1"${version}"`);
  if (next === s) {
    console.error(`[bump] ${file}: 未找到 version 字段`);
    process.exit(1);
  }
  writeFileSync(file, next);
  console.log(`[bump] ${file} -> ${version}`);
}

const cargo = readFileSync("src-tauri/Cargo.toml", "utf8");
const cargoNext = cargo.replace(/^version = "[^"]+"/m, `version = "${version}"`);
if (cargoNext === cargo) {
  console.error("[bump] src-tauri/Cargo.toml: 未找到 package version");
  process.exit(1);
}
writeFileSync("src-tauri/Cargo.toml", cargoNext);
console.log(`[bump] src-tauri/Cargo.toml -> ${version}`);

// 根 workspace 的版本是 gitwave-core 与 GPUI Kit 应用共用的唯一一处。
const workspace = readFileSync("Cargo.toml", "utf8");
const workspaceNext = workspace.replace(/^version = "[^"]+"/m, `version = "${version}"`);
if (workspaceNext === workspace) {
  console.error("[bump] Cargo.toml: 未找到 workspace package version");
  process.exit(1);
}
writeFileSync("Cargo.toml", workspaceNext);
console.log(`[bump] Cargo.toml -> ${version}`);

execSync("cargo update -p gitwave --manifest-path src-tauri/Cargo.toml", {
  stdio: "inherit",
});
// cargo metadata 只做过期解析，把两个 workspace 里路径依赖的新版本写进锁文件。
// 它的输出很大且这里用不上，直接丢弃，否则会超过 execSync 的缓冲上限。
const discardOutput = { stdio: ["ignore", "ignore", "inherit"] };
execSync("cargo metadata --manifest-path src-tauri/Cargo.toml --format-version 1", discardOutput);
execSync("cargo metadata --format-version 1", discardOutput);
console.log(`[bump] Cargo.lock 已刷新，全部版本 -> ${version}`);
