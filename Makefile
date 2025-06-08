# Define shell for consistent behavior
SHELL := /bin/bash

# Get current git commit hash (short version)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Get the closest git tag, or default to v0.0.0-dev if no tags are found
GIT_VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0-dev")

.PHONY: build run clean

check:
	@echo "COMMIT: $(GIT_COMMIT)"
	@echo "VERSON: $(GIT_VERSION)"

build:
	@echo "Building Docker image: server"
	# Pass build arguments to the Dockerfile
	docker compose build \
		--build-arg GIT_VERSION=$(GIT_VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		server
	docker compose up -d server

# run: build
# 	@echo "Running Docker Compose services..."
# 	docker compose up -d

# clean:
# 	@echo "Stopping and removing Docker Compose services..."
# 	docker compose down --volumes --rmi all

# # Add a target to clean just the image (e.g., if you only want to rebuild)
# clean-image:
# 	@echo "Removing Docker image: $(IMAGE_TAG)"
# 	docker rmi $(IMAGE_TAG) || true # Use || true to prevent error if image doesn't exist