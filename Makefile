BINARY := gocker
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.2.0")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
DOCKER_REPO := docker.io/adyjay/gocker
TEMPLATE_DIR := templates/claude

.PHONY: build install test lint clean template-build template-push

build:
	go build $(LDFLAGS) -o $(BINARY) .

install: build
	sudo cp $(BINARY) /usr/local/bin/
	sudo codesign -s - /usr/local/bin/$(BINARY)

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -f $(BINARY)

template-build:
	container build \
		-t $(DOCKER_REPO):claude-$(VERSION) \
		$(TEMPLATE_DIR)
	container image tag $(DOCKER_REPO):claude-$(VERSION) $(DOCKER_REPO):claude-latest

template-push: template-build
	container image push $(DOCKER_REPO):claude-$(VERSION)
	container image push $(DOCKER_REPO):claude-latest
