# Local build for trying the CLI without go install.
#
#	make
#	./cloudns auth status
#
#	make run ARGS="zone list"
#	make run ARGS="domain list example.com"

BINARY := ./cloudns

.DEFAULT_GOAL := build

.PHONY: build run test clean build-install

build:
	go build -o $(BINARY) ./cmd/cloudns

run: build
	$(BINARY) $(ARGS)

test:
	go test ./...

clean:
	rm -f $(BINARY)


build-install: build ## Build and install the CLI
	$(BINARY) install
