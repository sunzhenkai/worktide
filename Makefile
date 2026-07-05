# WorkTide Makefile
# 统一构建入口；Windows 用户可改用 scripts/*.ps1 或直接 `go run ./cmd/worktide`。

BINARY   := worktide
PKG      := ./...
CMD      := ./cmd/worktide
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")
LDFLAGS  := -X github.com/sunzhenkai/worktide/internal/app.Version=$(VERSION)

.PHONY: all build run test fmt vet lint tidy install clean help

## all: 构建并运行检查（默认）
all: build vet test

## build: 编译二进制到 bin/<BINARY>
build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(CMD)

## run: 直接运行（开发用）
run:
	go run $(CMD)

## test: 运行全部单元测试
test:
	go test $(PKG)

## fmt: 格式化源码
fmt:
	gofmt -s -w .

## vet: 静态检查
vet:
	go vet $(PKG)

## lint: 运行 golangci-lint（如已安装）
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "未安装 golangci-lint，跳过（安装: https://golangci-lint.run）"; exit 0; }
	golangci-lint run $(PKG)

## tidy: 整理依赖
tidy:
	go mod tidy

## install: 安装到 GOBIN
install:
	go install -ldflags "$(LDFLAGS)" $(CMD)

## clean: 清理构建产物
clean:
	rm -rf bin/

## help: 显示本帮助
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | sed 's/://' | awk -F' ' '{printf "  \033[36m%-10s\033[0m %s\n", $$1, substr($$0, index($$0,$$2))}'
