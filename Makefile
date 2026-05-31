BIN := retrox

# Build the React frontend into ./web (consumed by //go:embed in main.go).
.PHONY: web
web:
	$(MAKE) -C retrox-web build-web

# Full build: frontend then the Go binary.
.PHONY: build
build: web
	go build -o $(BIN) .

.PHONY: run
run: build
	./$(BIN)

# Dev — backend (:50000) + frontend hot-reload (:50001) in parallel, with
# coloured prefixed output. Installs frontend deps on first run. Ctrl-C
# kills both process trees cleanly.
.PHONY: dev
dev:
	@./scripts/dev.sh

.PHONY: test
test:
	go test ./...

.PHONY: package
package: build
	./scripts/package.sh

.PHONY: tidy
tidy:
	go mod tidy
