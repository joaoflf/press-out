TAILWINDCSS ?= npx tailwindcss

.PHONY: build sqlc-generate tailwind-build go-build test run dev setup check-deps

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

setup:
	npm ci

check-deps:
	@echo "Checking dependencies..."
	@which ffmpeg > /dev/null 2>&1 && echo "  ffmpeg: $$(ffmpeg -version 2>&1 | head -1)" || echo "  ffmpeg: NOT FOUND (required for video processing)"
	@which ffprobe > /dev/null 2>&1 && echo "  ffprobe: $$(ffprobe -version 2>&1 | head -1)" || echo "  ffprobe: NOT FOUND (required for video analysis)"
	@which sqlc > /dev/null 2>&1 && echo "  sqlc: $$(sqlc version 2>&1)" || echo "  sqlc: NOT FOUND"
	@which go > /dev/null 2>&1 && echo "  go: $$(go version 2>&1)" || echo "  go: NOT FOUND"
