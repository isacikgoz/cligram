# Lint runs a pinned golangci-lint built with this module's Go, so it reads
# the same standard library the code is compiled against.
GOLANGCI_LINT = go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0

.PHONY: test lint golden fuzz pixels playground serve

test:
	go test -race ./...

lint:
	$(GOLANGCI_LINT) run ./...

# golden rewrites testdata/*.golden from what the code draws now; look at
# the diff before committing it.
golden:
	go test ./... -update

# fuzz draws random diagrams and reads every drawing back, flowcharts and
# then sequence diagrams, each for FUZZTIME; a failing input lands in the
# package's testdata/fuzz/ and runs in every test after. A worker that
# crashes leaves its trace beside it, in crash-<pid>.txt (internal/crash).
FUZZTIME ?= 2m
fuzz:
	@status=0; \
	go test -run '^$$' -fuzz FuzzDrawings -fuzztime $(FUZZTIME) . && \
	go test -run '^$$' -fuzz FuzzDrawings -fuzztime $(FUZZTIME) ./sequence || status=$$?; \
	find . -path '*/testdata/fuzz/crash-*.txt' -empty -delete; \
	exit $$status

# pixels renders cases in a real terminal engine (xterm.js in headless
# Chromium, in real fonts) and checks the screenshots cell by cell. It
# needs Node; the first run fetches packages, fonts and a browser.
PIXELS_CASES ?= 40
pixels:
	cd harness/pixels && npm ci --silent && ./fonts.sh && npx playwright install chromium
	rm -rf harness/pixels/out
	CLIGRAM_PIXELS_OUT=harness/pixels/out CLIGRAM_PIXELS_CASES=$(PIXELS_CASES) go test -count=1 -run TestExportPixelCases .
	cd harness/pixels && node render.mjs
	go test -count=1 ./internal/pixels/

# playground builds the browser playground into playground/dist: cligram
# compiled to WebAssembly, the Go runtime's loader, and the page.
playground:
	rm -rf playground/dist && mkdir -p playground/dist
	GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o playground/dist/cligram.wasm ./playground
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" playground/index.html playground/worker.js playground/dist/

# serve builds the playground and serves it on http://localhost:$(PORT).
PORT ?= 8418
serve: playground
	@echo "the playground: http://localhost:$(PORT)"
	cd playground/dist && python3 -m http.server $(PORT) --bind 127.0.0.1
