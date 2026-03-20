BINARY := gocker
VERSION := 0.1.0
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build install test lint clean

build:
	go build $(LDFLAGS) -o $(BINARY) .

install: build
	sudo cp $(BINARY) /usr/local/bin/

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -f $(BINARY)
