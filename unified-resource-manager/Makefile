.PHONY: build test clean docker-build docker-push deploy

# 变量定义
VERSION ?= v1.0.0
IMAGE_NAME ?= flexai/unified-resource-manager
REGISTRY ?= docker.io
GO_VERSION ?= 1.21

# 构建二进制文件
build:
	@echo "Building unified-resource-manager..."
	@go build -o bin/unified-resource-manager ./cmd/main.go

# 运行测试
test:
	@echo "Running tests..."
	@go test -v ./...

# 清理构建产物
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -rf dist/

# 构建 Docker 镜像
docker-build:
	@echo "Building Docker image..."
	@docker build -t $(REGISTRY)/$(IMAGE_NAME):$(VERSION) .
	@docker tag $(REGISTRY)/$(IMAGE_NAME):$(VERSION) $(REGISTRY)/$(IMAGE_NAME):latest

# 推送 Docker 镜像
docker-push:
	@echo "Pushing Docker image..."
	@docker push $(REGISTRY)/$(IMAGE_NAME):$(VERSION)
	@docker push $(REGISTRY)/$(IMAGE_NAME):latest

# 部署到 Kubernetes
deploy:
	@echo "Deploying to Kubernetes..."
	@kubectl apply -f deploy/k8s/

# 格式化代码
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# 代码检查
lint:
	@echo "Running linter..."
	@golangci-lint run ./...

# 生成文档
docs:
	@echo "Generating documentation..."
	@go doc -all ./... > docs/API_REFERENCE.md

# 运行本地开发服务器
run:
	@echo "Running local server..."
	@go run ./cmd/main.go -port 8080

# 安装依赖
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# 完整构建流程
all: clean deps fmt test build
