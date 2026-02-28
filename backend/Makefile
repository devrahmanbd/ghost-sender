.PHONY: help build run test clean deps migrate docker-build docker-up

# Variables
APP_NAME=email-campaign-system
BUILD_DIR=bin
MAIN_SERVER=cmd/server/main.go
MAIN_CLI=cmd/cli/main.go

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

deps: ## Install dependencies
	go mod download
	go mod tidy

build: ## Build the application
	@echo "Building server..."
	go build -o $(BUILD_DIR)/server $(MAIN_SERVER)
	@echo "Building CLI..."
	go build -o $(BUILD_DIR)/cli $(MAIN_CLI)

run-server: ## Run the server
	go run $(MAIN_SERVER)

run-cli: ## Run the CLI
	go run $(MAIN_CLI)

test: ## Run tests
	go test -v -race -coverprofile=coverage.out ./...

test-coverage: test ## Run tests with coverage report
	go tool cover -html=coverage.out

migrate-up: ## Run database migrations up
	./scripts/migrate.sh up

migrate-down: ## Run database migrations down
	./scripts/migrate.sh down

clean: ## Clean build artifacts
	rm -rf $(BUILD_DIR)
	rm -f coverage.out

docker-build: ## Build Docker image
	docker build -t $(APP_NAME):latest .

docker-up: ## Start Docker containers
	docker-compose up -d

docker-down: ## Stop Docker containers
	docker-compose down

docker-logs: ## View Docker logs
	docker-compose logs -f

fmt: ## Format code
	go fmt ./...

lint: ## Run linter
	golangci-lint run

.DEFAULT_GOAL := help
