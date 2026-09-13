#!/usr/bin/env node
//! 统一更新全仓库版本号，并刷新 Cargo.lock。
//!
//! 版本号声明位置：
//!   - package.json              —— pnpm 根包
//!   - src-tauri/tauri.conf.json —— 安装包版本
//!   - src-tauri/Cargo.toml      —— Rust crate 版本
//! 锁文件：Cargo.lock 的 gitwave 条目由 `cargo update -p gitwave` 刷新；
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

execSync("cargo update -p gitwave --manifest-path src-tauri/Cargo.toml", { stdio: "inherit" });
console.log(`[bump] Cargo.lock 已刷新，全部版本 -> ${version}`);
