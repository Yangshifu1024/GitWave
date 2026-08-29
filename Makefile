CARGO := cargo --manifest-path src-tauri/Cargo.toml

.DEFAULT_GOAL := help
.PHONY: test format fmt-check lint check build dev help

format: ## 格式化前端 + Rust(会改文件)
	npm run format
	$(CARGO) fmt

fmt-check: ## 只校验格式,不改文件(对齐 CI)
	npx prettier --check .
	$(CARGO) fmt -- --check

test: ## 前端 + Rust 全部测试
	npm test
	$(CARGO) test --all-targets

lint: ## eslint + clippy + typecheck + cargo check
	npm run lint
	$(CARGO) clippy --all-targets -- -D warnings
	npm run typecheck
	$(CARGO) check --all-targets

check: fmt-check lint test ## 提交前自查:只校验不改文件,等价 CI

build: format lint test ## 全量检查后构建发布包
	npm run tauri build

dev: ## 启动开发模式(不做前置检查)
	npm run tauri dev

help: ## 显示可用命令
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'
