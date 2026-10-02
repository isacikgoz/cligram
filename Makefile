# Lint runs a pinned golangci-lint built with this module's Go, so it reads
# the same standard library the code is compiled against.
GOLANGCI_LINT = go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0

.PHONY: test lint golden fuzz pixels

test:
	go test -race ./...

lint:
	$(GOLANGCI_LINT) run ./...

# golden rewrites testdata/*.golden from what the code draws now; look at
# the diff before committing it.
golden:
	go test ./... -update

# fuzz draws random diagrams and reads every drawing back, for FUZZTIME;
# a failing input lands in testdata/fuzz/ and runs in every test after.
FUZZTIME ?= 2m
fuzz:
	go test -run '^$$' -fuzz FuzzDrawings -fuzztime $(FUZZTIME) .

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
