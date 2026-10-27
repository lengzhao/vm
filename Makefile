# VM is a Go library; examples are the runnable entry points.
GOCMD ?= go
TEST_TIMEOUT ?= 5m
TARGET_ARCH ?= amd64

all: build

build:
	$(GOCMD) build ./...

deps:
	$(GOCMD) mod tidy

fmt:
	$(GOCMD) fmt ./...

vet:
	$(GOCMD) vet ./...

test:
	$(GOCMD) test -p 1 -timeout $(TEST_TIMEOUT) ./...

test-race:
	$(GOCMD) test -race -p 1 -timeout $(TEST_TIMEOUT) ./...

# Recipes are sequential even when the caller uses make -j.
check:
	$(MAKE) test
	$(MAKE) test-race
	$(MAKE) vet
	git diff --check

test-cover:
	$(GOCMD) test -p 1 -timeout $(TEST_TIMEOUT) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

clean:
	$(GOCMD) clean ./...
	rm -f coverage.out coverage.html

install:
	$(GOCMD) install .

run:
	$(GOCMD) run ./examples/basic

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(TARGET_ARCH) $(GOCMD) build ./...

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=$(TARGET_ARCH) $(GOCMD) build ./...

build-mac:
	CGO_ENABLED=0 GOOS=darwin GOARCH=$(TARGET_ARCH) $(GOCMD) build ./...

docs:
	@echo "请查看 README.md 和 docs/plugins.md"

help:
	@echo "build / run / install: 构建全部包 / 运行 basic 示例 / 安装库"
	@echo "test / test-race / check: 普通测试 / race / 顺序完整验证"
	@echo "test-cover / fmt / vet / clean / deps / docs"
	@echo "build-linux / build-windows / build-mac: 交叉编译校验，TARGET_ARCH 默认 amd64"

.PHONY: all build deps fmt vet test test-race check test-cover clean install run build-linux build-windows build-mac docs help
