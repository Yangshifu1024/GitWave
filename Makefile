CARGO := cargo
CARGO_MANIFEST := --manifest-path src-tauri/Cargo.toml

.DEFAULT_GOAL := help
.PHONY: test format fmt-check lint check build dev native-dev native-test help

NATIVE_CARGO := $(CARGO) build $(CARGO_MANIFEST) --locked --features native-preview --bin gitwave-native-core
NATIVE_TARGET_DIR := $(if $(CARGO_TARGET_DIR),$(abspath $(CARGO_TARGET_DIR)),$(CURDIR)/src-tauri/target)
NATIVE_CORE := $(NATIVE_TARGET_DIR)/debug/gitwave-native-core$(if $(filter Windows_NT,$(OS)),.exe)

format: ## 格式化前端 + Rust(会改文件)
	pnpm format
	$(CARGO) fmt $(CARGO_MANIFEST)

fmt-check: ## 只校验格式,不改文件(对齐 CI)
	pnpm exec prettier --check .
	$(CARGO) fmt $(CARGO_MANIFEST) -- --check

test: ## 前端 + Rust 全部测试
	pnpm test
	$(CARGO) test $(CARGO_MANIFEST) --all-targets

lint: ## Tauri version compatibility + eslint + clippy + typecheck + cargo check
	pnpm check:tauri-versions
	pnpm lint
	$(CARGO) clippy $(CARGO_MANIFEST) --all-targets -- -D warnings
	pnpm typecheck
	$(CARGO) check $(CARGO_MANIFEST) --all-targets

check: fmt-check lint test ## 提交前自查:只校验不改文件,等价 CI

build: format lint test ## 全量检查后构建发布包
	pnpm tauri build

dev: ## 启动开发模式(不做前置检查)
	pnpm tauri dev

native-dev: ## 启动 NSGo 原生 History/diff 预览
	CARGO_TARGET_DIR="$(NATIVE_TARGET_DIR)" $(NATIVE_CARGO)
	GITWAVE_NATIVE_CORE="$(NATIVE_CORE)" go -C native run .

native-test: ## 用真实 Git 仓库验证原生 History/diff
	CARGO_TARGET_DIR="$(NATIVE_TARGET_DIR)" $(NATIVE_CARGO)
	GITWAVE_NATIVE_CORE="$(NATIVE_CORE)" go -C native test ./...

help: ## 显示可用命令
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'
