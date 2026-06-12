.PHONY: build install uninstall test lint clean

BINARY  := cerebro
INSTALL := /usr/local/bin/$(BINARY)

build:
	go build -tags fts5 -o $(BINARY) ./cmd/cerebro

install: build
	cp $(BINARY) $(INSTALL)
	@echo "Installed $(BINARY) to $(INSTALL)"

uninstall:
	rm -f $(INSTALL)
	@echo "Removed $(INSTALL)"

test:
	go test -tags fts5 ./... -race

test-cover:
	go test -tags fts5 ./... -race -coverprofile=coverage.out
	go tool cover -func=coverage.out

lint:
	golangci-lint run

clean:
	rm -f $(BINARY) coverage.out
