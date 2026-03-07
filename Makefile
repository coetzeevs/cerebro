.PHONY: build install uninstall test lint clean

BINARY  := cerebro
INSTALL := /usr/local/bin/$(BINARY)

build:
	go build -o $(BINARY) ./cmd/cerebro

install: build
	cp $(BINARY) $(INSTALL)
	@echo "Installed $(BINARY) to $(INSTALL)"

uninstall:
	rm -f $(INSTALL)
	@echo "Removed $(INSTALL)"

test:
	go test ./... -race

test-cover:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -func=coverage.out

lint:
	golangci-lint run

clean:
	rm -f $(BINARY) coverage.out
