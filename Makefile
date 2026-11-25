CARGO := cargo
WORKSPACE := --workspace --all-targets

.DEFAULT_GOAL := help
.PHONY: test format fmt-check lint check build dev help

format: ## 格式化 Rust 源码(会改文件)
	$(CARGO) fmt --all

fmt-check: ## 只校验格式,不改文件(对齐 CI)
	$(CARGO) fmt --all -- --check

test: ## 运行全部 Rust 测试
	$(CARGO) test $(WORKSPACE)

lint: ## clippy 静态检查
	$(CARGO) clippy $(WORKSPACE) -- -D warnings

check: fmt-check lint test ## 提交前自查:只校验不改文件,等价 CI

build: check ## 全量检查后构建应用(本机调试版)
	$(CARGO) build -p gitwave

dev: ## 启动开发模式(不做前置检查)
	$(CARGO) run -p gitwave

help: ## 显示可用命令
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'
