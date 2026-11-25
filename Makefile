GO := go
NSGO := $(GO) -C cmd/gitwave tool nsgo
NSGO_TARGET = $(subst darwin,macos,$(shell $(GO) env GOOS))_$(subst amd64,x64,$(shell $(GO) env GOARCH))

.DEFAULT_GOAL := help
.PHONY: format fmt-check lint test test-race check build dev version bump help \
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

native-dev: dev
native-test: test

native-git-test: ## 用本机 Git 验证 Go 仓库适配层
	bash scripts/check-go-test-tools.sh
	$(GO) test -count=1 ./internal/gitcli

help: ## 显示可用命令
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-19s\033[0m %s\n", $$1, $$2}'
