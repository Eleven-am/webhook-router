.PHONY: build run test clean docker-build docker-run dev setup docker-multiarch docker-push

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=webhook-router
BINARY_UNIX=$(BINARY_NAME)_unix

# Docker parameters - can be overridden
DOCKER_IMAGE ?= $(shell read -p "Enter Docker image name (e.g., username/webhook-router): " image; echo $$image)
DOCKER_TAG ?= latest
DOCKER_PLATFORMS = linux/amd64,linux/arm64
TIMESTAMP = $(shell date +"%Y-%m-%d-%H-%M")

# Build the application
build:
	CGO_ENABLED=1 $(GOBUILD) -o $(BINARY_NAME) -v

# Run the application
run: build
	./$(BINARY_NAME)

# Run in development mode with hot reload (requires air)
dev:
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "Installing air for hot reload..."; \
		go install github.com/cosmtrek/air@latest; \
		air; \
	fi

# Test the application
test:
	$(GOTEST) -v ./...

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Setup development environment (simplified - no web/static needed)
setup: deps
	@echo "Setting up development environment..."
	@echo "Development environment ready!"
	@echo "Note: Web assets are embedded in the binary"

# Build for Linux
build-linux:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v

# Traditional Docker commands (single architecture)
docker-build:
	docker build -t webhook-router .

docker-run: docker-build
	docker run -p 8080:8080 -v webhook_data:/data webhook-router

# Multi-architecture Docker build setup
docker-setup:
	@echo "Setting up Docker BuildX for multi-architecture builds..."
	@docker buildx create --name multiarch-builder --use 2>/dev/null || echo "Builder already exists"
	@docker buildx inspect --bootstrap

# Interactive Docker image name prompt
docker-image-prompt:
	@if [ -z "$(DOCKER_IMAGE_NAME)" ]; then \
		read -p "Enter Docker image name (e.g., username/webhook-router): " DOCKER_IMAGE_NAME; \
		export DOCKER_IMAGE_NAME; \
	fi

# Build multi-architecture Docker images locally
docker-multiarch: docker-setup
	@if [ -z "$(DOCKER_IMAGE_NAME)" ]; then \
		read -p "Enter Docker image name (e.g., username/webhook-router): " DOCKER_IMAGE_NAME; \
	else \
		DOCKER_IMAGE_NAME=$(DOCKER_IMAGE_NAME); \
	fi; \
	echo "Building multi-architecture images for: $$DOCKER_IMAGE_NAME"; \
	docker buildx build \
		--platform $(DOCKER_PLATFORMS) \
		--build-arg IMAGE_NAME="$$DOCKER_IMAGE_NAME" \
		--build-arg IMAGE_TIMESTAMP="$(TIMESTAMP)" \
		-t "$$DOCKER_IMAGE_NAME:$(DOCKER_TAG)" \
		-t "$$DOCKER_IMAGE_NAME:$(TIMESTAMP)" \
		--load \
		.

# Build and push multi-architecture Docker images
docker-push: docker-setup
	@if [ -z "$(DOCKER_IMAGE_NAME)" ]; then \
		read -p "Enter Docker image name (e.g., username/webhook-router): " DOCKER_IMAGE_NAME; \
	else \
		DOCKER_IMAGE_NAME=$(DOCKER_IMAGE_NAME); \
	fi; \
	echo "Building and pushing multi-architecture images for: $$DOCKER_IMAGE_NAME"; \
	docker buildx build \
		--platform $(DOCKER_PLATFORMS) \
		--build-arg IMAGE_NAME="$$DOCKER_IMAGE_NAME" \
		--build-arg IMAGE_TIMESTAMP="$(TIMESTAMP)" \
		-t "$$DOCKER_IMAGE_NAME:$(DOCKER_TAG)" \
		-t "$$DOCKER_IMAGE_NAME:$(TIMESTAMP)" \
		--push \
		.

# Build and push with custom tags
docker-push-tags: docker-setup
	@if [ -z "$(DOCKER_IMAGE_NAME)" ]; then \
		read -p "Enter Docker image name (e.g., username/webhook-router): " DOCKER_IMAGE_NAME; \
	else \
		DOCKER_IMAGE_NAME=$(DOCKER_IMAGE_NAME); \
	fi; \
	if [ -z "$(TAGS)" ]; then \
		read -p "Enter tags separated by space (e.g., latest dev v1.0): " TAGS; \
	fi; \
	echo "Building and pushing with tags: $$TAGS"; \
	TAG_ARGS=""; \
	for tag in $$TAGS; do \
		TAG_ARGS="$$TAG_ARGS -t $$DOCKER_IMAGE_NAME:$$tag -t $$DOCKER_IMAGE_NAME:$$tag-$(TIMESTAMP)"; \
	done; \
	docker buildx build \
		--platform $(DOCKER_PLATFORMS) \
		--build-arg IMAGE_NAME="$$DOCKER_IMAGE_NAME" \
		--build-arg IMAGE_TIMESTAMP="$(TIMESTAMP)" \
		$$TAG_ARGS \
		--push \
		.

# Build production images with optimization
docker-production: docker-setup
	@if [ -z "$(DOCKER_IMAGE_NAME)" ]; then \
		read -p "Enter Docker image name (e.g., username/webhook-router): " DOCKER_IMAGE_NAME; \
	else \
		DOCKER_IMAGE_NAME=$(DOCKER_IMAGE_NAME); \
	fi; \
	echo "Building production images for: $$DOCKER_IMAGE_NAME"; \
	docker buildx build \
		--platform $(DOCKER_PLATFORMS) \
		--build-arg IMAGE_NAME="$$DOCKER_IMAGE_NAME" \
		--build-arg IMAGE_TIMESTAMP="$(TIMESTAMP)" \
		--build-arg CGO_ENABLED=1 \
		--build-arg LDFLAGS="-w -s" \
		-t "$$DOCKER_IMAGE_NAME:production" \
		-t "$$DOCKER_IMAGE_NAME:production-$(TIMESTAMP)" \
		--push \
		.

# Docker Compose commands
up:
	docker-compose up -d

down:
	docker-compose down

logs:
	docker-compose logs -f

restart:
	docker-compose restart

# Database operations
db-reset:
	rm -f webhook_router.db
	@echo "Database reset. It will be recreated on next run."

db-backup:
	@if [ -f webhook_router.db ]; then \
		cp webhook_router.db webhook_router_backup_$(shell date +%Y%m%d_%H%M%S).db; \
		echo "Database backed up successfully"; \
	else \
		echo "No database file found"; \
	fi

# Development helpers
fmt:
	$(GOCMD) fmt ./...

vet:
	$(GOCMD) vet ./...

lint:
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Generate API documentation
docs:
	@echo "Generating API documentation..."
	@if command -v swag > /dev/null; then \
		swag init; \
	else \
		echo "swag not installed. Install with: go install github.com/swaggo/swag/cmd/swag@latest"; \
	fi

# Install development tools
install-tools:
	$(GOGET) github.com/cosmtrek/air@latest
	$(GOGET) github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GOGET) github.com/swaggo/swag/cmd/swag@latest

# Production build
build-prod:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags="-w -s" -o $(BINARY_NAME) -v

# Health check
health:
	@curl -f http://localhost:8080/health || echo "Service is not healthy"

# Load test (requires wrk)
load-test:
	@if command -v wrk > /dev/null; then \
		wrk -t12 -c400 -d30s --timeout 10s http://localhost:8080/health; \
	else \
		echo "wrk not installed. Install with your package manager"; \
	fi

# Create example route (needs authentication)
example-route:
	@echo "Note: This requires authentication. Login to admin interface first."
	@echo "Creating example route via API..."
	curl -X POST http://localhost:8080/api/routes \
		-H "Content-Type: application/json" \
		-d '{ \
			"name": "GitHub Webhooks", \
			"endpoint": "github", \
			"method": "POST", \
			"queue": "github-events", \
			"routing_key": "github.events", \
			"filters": "{\"headers\": {\"X-GitHub-Event\": \"push\"}}", \
			"active": true \
		}' || echo "Failed - make sure you're logged in to admin interface"

# Test webhook
test-webhook:
	curl -X POST http://localhost:8080/webhook/github \
		-H "Content-Type: application/json" \
		-H "X-GitHub-Event: push" \
		-d '{"repository": {"name": "test-repo"}, "action": "opened"}'

# Test default webhook endpoint
test-webhook-default:
	curl -X POST http://localhost:8080/webhook \
		-H "Content-Type: application/json" \
		-d '{"test": "data", "timestamp": "'$(shell date -Iseconds)'"}'

# Docker cleanup
docker-clean:
	@echo "Cleaning up Docker resources..."
	@docker buildx prune -f
	@docker system prune -f
	@echo "Docker cleanup completed"

# Show Docker build info
docker-info:
	@echo "Docker BuildX Info:"
	@docker buildx ls
	@echo ""
	@echo "Available platforms:"
	@docker buildx inspect --bootstrap | grep Platforms || echo "No builder found"

# Quick start for development
quick-start:
	@echo "🚀 Starting Webhook Router..."
	@echo "1. Building application..."
	@make build
	@echo "2. Starting server..."
	@echo "📡 Server will be available at: http://localhost:8080"
	@echo "🔐 Default login: admin/admin"
	@echo "⚠️  You'll be prompted to change the password on first login"
	@echo ""
	@./$(BINARY_NAME)

# Show available endpoints
show-endpoints:
	@echo "📡 Available Endpoints:"
	@echo ""
	@echo "🔐 Authentication:"
	@echo "  GET  /login              - Login page"
	@echo "  POST /login              - Login form"
	@echo "  GET  /logout             - Logout"
	@echo "  GET  /change-password    - Change password page"
	@echo "  POST /change-password    - Change password form"
	@echo ""
	@echo "📊 Admin Interface:"
	@echo "  GET  /admin              - Main admin dashboard"
	@echo "  GET  /                   - Redirects to admin"
	@echo ""
	@echo "🔧 API Endpoints (require authentication):"
	@echo "  GET    /api/routes       - List all routes"
	@echo "  POST   /api/routes       - Create new route"
	@echo "  PUT    /api/routes/{id}  - Update route"
	@echo "  DELETE /api/routes/{id}  - Delete route"
	@echo "  POST   /api/routes/{id}/test - Test route"
	@echo "  GET    /api/stats        - Get statistics"
	@echo "  GET    /api/settings     - Get settings"
	@echo "  POST   /api/settings     - Update settings"
	@echo ""
	@echo "📥 Webhook Endpoints:"
	@echo "  *    /webhook            - Default webhook endpoint"
	@echo "  *    /webhook/{endpoint} - Custom webhook endpoint"
	@echo ""
	@echo "🏥 Health Check:"
	@echo "  GET  /health             - System health status"

# Show help
help:
	@echo "Available commands:"
	@echo ""
	@echo "🚀 Quick Start:"
	@echo "  quick-start   - Build and run with helpful info"
	@echo ""
	@echo "Development:"
	@echo "  build         - Build the application"
	@echo "  run           - Build and run the application"
	@echo "  dev           - Run in development mode with hot reload"
	@echo "  test          - Run tests"
	@echo "  clean         - Clean build artifacts"
	@echo "  deps          - Download dependencies"
	@echo "  setup         - Setup development environment"
	@echo ""
	@echo "Docker (Single Architecture):"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-run    - Build and run Docker container"
	@echo ""
	@echo "Docker (Multi-Architecture):"
	@echo "  docker-setup      - Setup Docker BuildX for multi-arch"
	@echo "  docker-multiarch  - Build multi-arch images locally"
	@echo "  docker-push       - Build and push multi-arch images"
	@echo "  docker-push-tags  - Build and push with custom tags"
	@echo "  docker-production - Build and push production images"
	@echo "  docker-clean      - Clean Docker resources"
	@echo "  docker-info       - Show Docker build information"
	@echo ""
	@echo "Docker Compose:"
	@echo "  up            - Start with docker-compose"
	@echo "  down          - Stop docker-compose"
	@echo "  logs          - Show docker-compose logs"
	@echo "  restart       - Restart docker-compose services"
	@echo ""
	@echo "Database:"
	@echo "  db-reset      - Reset database"
	@echo "  db-backup     - Backup database"
	@echo ""
	@echo "Development Tools:"
	@echo "  fmt           - Format Go code"
	@echo "  vet           - Run go vet"
	@echo "  lint          - Run golangci-lint"
	@echo "  install-tools - Install development tools"
	@echo ""
	@echo "Testing & Info:"
	@echo "  health            - Check service health"
	@echo "  example-route     - Create an example route"
	@echo "  test-webhook      - Send a test webhook to custom endpoint"
	@echo "  test-webhook-default - Send a test webhook to default endpoint"
	@echo "  show-endpoints    - Show all available endpoints"
	@echo "  load-test         - Run load test"
	@echo ""
	@echo "Usage Examples:"
	@echo "  make quick-start"
	@echo "  make docker-push DOCKER_IMAGE_NAME=username/webhook-router"
	@echo "  make docker-push-tags DOCKER_IMAGE_NAME=username/webhook-router TAGS='latest dev v1.0'"
	@echo "  DOCKER_IMAGE_NAME=username/webhook-router make docker-push"