# 智能合约虚拟机 Makefile

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Binary name
BINARY_NAME=vm
BINARY_UNIX=$(BINARY_NAME)_unix

# Default target
all: build

# Build the project
build:
	$(GOBUILD) -o $(BINARY_NAME) -v

# Install dependencies
deps:
	$(GOMOD) tidy

# Format source code
fmt:
	$(GOFMT) ./...

# Vet source code
vet:
	$(GOVET) ./...

# Run tests
test:
	$(GOTEST) -v ./...

# Run tests with coverage
test-cover:
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)
	rm -f coverage.out
	rm -f coverage.html

# Install the binary
install:
	$(GOBUILD) -o $(GOPATH)/bin/$(BINARY_NAME)

# Run the project
run:
	$(GOCMD) run .

# Build for different platforms
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME).exe -v

build-mac:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)_mac -v

# Documentation
docs:
	@echo "生成文档..."
	@echo "请查看 docs/ 目录下的文档"

# Help
help:
	@echo "可用的命令:"
	@echo "  all          - 构建项目 (默认)"
	@echo "  build        - 编译项目"
	@echo "  deps         - 安装依赖"
	@echo "  fmt          - 格式化代码"
	@echo "  vet          - 检查代码问题"
	@echo "  test         - 运行测试"
	@echo "  test-cover   - 运行测试并生成覆盖率报告"
	@echo "  clean        - 清理构建产物"
	@echo "  install      - 安装二进制文件"
	@echo "  run          - 运行项目"
	@echo "  build-linux  - 构建Linux版本"
	@echo "  build-windows - 构建Windows版本"
	@echo "  build-mac    - 构建Mac版本"
	@echo "  docs         - 文档信息"
	@echo "  help         - 显示此帮助信息"

.PHONY: all build deps fmt vet test test-cover clean install run build-linux build-windows build-mac docs help