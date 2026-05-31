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

# Dev: run the Go API and the rsbuild dev server (proxies /api) separately.
.PHONY: dev
dev:
	@echo "API:      go run ."
	@echo "Frontend: cd retrox-web && npm run dev   → http://localhost:50001"

.PHONY: tidy
tidy:
	go mod tidy
