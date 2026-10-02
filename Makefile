# Lint runs a pinned golangci-lint built with this module's Go, so it reads
# the same standard library the code is compiled against.
GOLANGCI_LINT = go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0

.PHONY: test lint golden

test:
	go test -race ./...

lint:
	$(GOLANGCI_LINT) run ./...

# golden rewrites testdata/*.golden from what the code draws now; look at
# the diff before committing it.
golden:
	go test ./... -update
