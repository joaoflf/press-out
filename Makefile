TAILWINDCSS ?= npx tailwindcss

.PHONY: build sqlc-generate tailwind-build go-build test run dev

build: sqlc-generate tailwind-build go-build

sqlc-generate:
	sqlc generate

tailwind-build:
	$(TAILWINDCSS) -i web/static/input.css -o web/static/output.css --minify

go-build:
	CGO_ENABLED=1 go build -o press-out ./cmd/press-out

test:
	CGO_ENABLED=1 go test ./...

run:
	CGO_ENABLED=1 go run ./cmd/press-out

dev:
	@echo "Run with air for hot-reload: air"
