# KeiRouter developer tasks.
#
# Common entrypoints:
#   make dev        run backend + frontend together (hot reload)
#   make backend    run only the Go backend
#   make frontend   run only the Vite dev server
#   make build      build backend binary + frontend assets
#   make test       run backend tests
#   make bootstrap  create an initial API key
#   make setup      one-command: install deps + run (no .env needed)

BACKEND_DIR := backend
FRONTEND_DIR := frontend
BIN := keirouter

.PHONY: dev backend frontend build build-backend build-frontend test vet hooks bootstrap install setup quickstart clean docker

## dev: run backend and frontend concurrently; Ctrl-C stops both.
##      The backend starts first; frontend waits until the backend is healthy
##      so the Vite proxy never hits ECONNREFUSED on cold start.
dev:
	@echo "Starting KeiRouter backend (:20180) and dashboard (:5180)…"
	@trap 'kill 0' INT TERM EXIT; \
	( cd $(BACKEND_DIR) && go run ./cmd/keirouter ) & \
	( \
		echo "⏳ Waiting for backend…"; \
		ready=0; \
		for i in $$(seq 1 30); do \
			if curl -sf http://127.0.0.1:20180/healthz >/dev/null 2>&1; then \
				echo "✅ Backend ready"; \
				ready=1; \
				break; \
			fi; \
			sleep 0.5; \
		done; \
		if [ "$$ready" -ne 1 ]; then \
			echo "Backend did not become healthy on http://127.0.0.1:20180/healthz"; \
			exit 1; \
		fi; \
		cd $(FRONTEND_DIR) && npm run dev \
	) & \
	wait

## backend: run only the Go backend.
backend:
	cd $(BACKEND_DIR) && go run ./cmd/keirouter

## frontend: run only the Vite dev server.
frontend:
	cd $(FRONTEND_DIR) && npm run dev

## install: install frontend dependencies and download Go modules.
install:
	cd $(FRONTEND_DIR) && npm install
	cd $(BACKEND_DIR) && go mod download

## build: build the backend binary and the frontend assets.
build: build-frontend build-backend

build-backend:
	cd $(BACKEND_DIR) && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.Version=$$(git describe --tags --always --dirty 2>/dev/null || echo dev)" -o ../$(BIN) ./cmd/keirouter

build-frontend:
	cd $(FRONTEND_DIR) && npm run build

## hooks: install git pre-push hook (runs typecheck + vet before push).
hooks:
	@mkdir -p "$(shell git rev-parse --git-dir)/hooks"
	@cp scripts/hooks/pre-push "$(shell git rev-parse --git-dir)/hooks/pre-push"
	@chmod +x "$(shell git rev-parse --git-dir)/hooks/pre-push"
	@echo "✅ pre-push hook installed"

## test: run the backend test suite.
test:
	cd $(BACKEND_DIR) && go test ./...

## vet: run static analysis.
vet:
	cd $(BACKEND_DIR) && go vet ./...

## bootstrap: create an initial API key (printed once).
bootstrap:
	cd $(BACKEND_DIR) && go run ./cmd/keirouter -bootstrap

## docker: build the production image.
docker:
	docker build -f deploy/Dockerfile -t keirouter:latest .

## clean: remove build artifacts.
clean:
	rm -f $(BIN)
	rm -rf $(FRONTEND_DIR)/dist

## setup: install deps (if needed) then start dev servers.
##        Zero config — no .env required for local use.
##        Usage: make setup
setup:
	@echo "🚀 KeiRouter — one-command setup"
	@test -d $(FRONTEND_DIR)/node_modules || ( echo "📦 Installing frontend deps…" && cd $(FRONTEND_DIR) && npm ci --quiet )
	@cd $(BACKEND_DIR) && go mod download 2>/dev/null || true
	@echo ""
	@echo "✅ Dependencies ready. Starting dev servers…"
	@echo "   Backend  → http://localhost:20180"
	@echo "   Dashboard→ http://localhost:5180"
	@echo "   Password → keirouter"
	@echo ""
	@$(MAKE) dev

## quickstart: alias for setup.
quickstart: setup
