BINARY := gocker
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.2.0")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
DOCKER_REPO := docker.io/adyjay/gocker

.PHONY: build build-linux install test test-integration test-all lint clean smoke benchmark \
	template-build-claude template-push-claude \
	template-build-base template-push-base \
	template-build template-push

build:
	go build $(LDFLAGS) -o $(BINARY) .

build-linux:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)-linux .

install: build
	sudo cp $(BINARY) /usr/local/bin/
	sudo codesign -s - /usr/local/bin/$(BINARY)

test:
	go test ./...

test-integration:  ## Run integration tests (requires real container runtime)
	go test -tags integration -timeout 5m -v ./...

test-all:  ## Run unit + integration tests
	go test ./...
	go test -tags integration -timeout 5m -v ./...

lint:
	golangci-lint run ./...

clean:
	rm -f $(BINARY) $(BINARY)-linux

# --- Claude template ---

template-build-claude:
	container build \
		-t $(DOCKER_REPO):claude-$(VERSION) \
		templates/claude
	container image tag $(DOCKER_REPO):claude-$(VERSION) $(DOCKER_REPO):claude-latest

template-push-claude: template-build-claude
	container image push $(DOCKER_REPO):claude-$(VERSION)
	container image push $(DOCKER_REPO):claude-latest

# --- Base template (shared VM image) ---

template-build-base: build-linux
	cp $(BINARY)-linux templates/base/gocker
	container build \
		-t $(DOCKER_REPO):base-$(VERSION) \
		templates/base
	container image tag $(DOCKER_REPO):base-$(VERSION) $(DOCKER_REPO):base-latest
	rm -f templates/base/gocker

template-push-base: template-build-base
	container image push $(DOCKER_REPO):base-$(VERSION)
	container image push $(DOCKER_REPO):base-latest

# --- All templates ---

template-build: template-build-claude template-build-base
template-push: template-push-claude template-push-base

smoke: install
	@bash test/smoke.sh

benchmark: install
	@bash test/benchmark.sh
