GO := go
CARGO := cargo
CARGO_MANIFEST := --manifest-path src-tauri/Cargo.toml
NSGO := $(GO) -C cmd/gitwave tool nsgo
NSGO_TARGET = $(subst darwin,macos,$(shell $(GO) env GOOS))_$(subst amd64,x64,$(shell $(GO) env GOARCH))

.DEFAULT_GOAL := help
.PHONY: format fmt-check lint test test-race check build dev version bump help \
	legacy-format legacy-fmt-check legacy-lint legacy-test legacy-check legacy-build legacy-dev \
	native-dev native-test native-git-test

format: ## 格式化 Go 源码
	bash scripts/go-format.sh --write cmd internal

fmt-check: ## 检查 Go 格式
	bash scripts/go-format.sh cmd internal

lint: ## 运行 go vet
	$(GO) vet ./...

test: ## 运行 Go 测试
	bash scripts/check-go-test-tools.sh
	$(GO) test ./...

test-race: ## 运行 Go 竞态测试
	bash scripts/check-go-test-tools.sh
	$(GO) test -race ./...

check: fmt-check lint test ## 检查 Go 格式、静态分析和测试

build: check ## 检查后构建本机 NSGo 桌面应用
	$(NSGO) build -target $(NSGO_TARGET) -channel none

dev: ## 启动 NSGo 开发模式
	$(NSGO) dev -target native

version: ## 打印 NSGo 配置版本
	@$(GO) run ./cmd/release-version

bump: ## 用 VERSION=X.Y.Z 修改 NSGo 配置版本
	@test -n "$(VERSION)" || { echo 'Usage: make bump VERSION=X.Y.Z' >&2; exit 1; }
	$(GO) run ./cmd/release-version "$(VERSION)"

legacy-format: ## 格式化 Tauri 偏好导出版
	pnpm format
	$(CARGO) fmt $(CARGO_MANIFEST)

legacy-fmt-check: ## 检查 Tauri 偏好导出版格式
	pnpm exec prettier --check .
	$(CARGO) fmt $(CARGO_MANIFEST) -- --check

legacy-lint: ## 检查 Tauri 偏好导出版
	pnpm check:tauri-versions
	pnpm lint
	$(CARGO) clippy $(CARGO_MANIFEST) --all-targets -- -D warnings
	pnpm typecheck
	$(CARGO) check $(CARGO_MANIFEST) --all-targets

legacy-test: ## 运行 Tauri 偏好导出版测试
	pnpm test
	$(CARGO) test $(CARGO_MANIFEST) --all-targets

legacy-check: legacy-fmt-check legacy-lint legacy-test ## 运行 Tauri 偏好导出版的全部检查

legacy-build: legacy-check ## 构建 Tauri 偏好导出版
	pnpm tauri build

legacy-dev: ## 启动 Tauri 偏好导出版开发模式
	pnpm tauri dev

native-dev: dev
native-test: test

native-git-test: ## 用本机 Git 验证 Go 仓库适配层
	bash scripts/check-go-test-tools.sh
	$(GO) test -count=1 ./internal/gitcli

help: ## 显示可用命令
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-19s\033[0m %s\n", $$1, $$2}'
