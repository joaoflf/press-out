# Story 1.1: Project Initialization & Landing Page

Status: done

## Story

As a lifter,
I want to open press-out in my mobile browser and see the lift list home page,
so that I have a starting point for managing my training videos.

## Acceptance Criteria (BDD)

1. **Given** the project is built and running, **When** the lifter navigates to the root URL in Chrome mobile, **Then** the lift list page renders with the DaisyUI custom theme (warm white background #FAFAF8, dark charcoal text #2D2D2D, system font stack), **And** the page displays an empty state when no lifts exist, **And** an upload button is visible (sage accent #8BA888, white text, full-width, h-12), **And** the page loads within 1 second (NFR2)

2. **Given** the project structure, **When** the developer runs `make build`, **Then** the Go binary compiles, Tailwind CSS compiles via standalone CLI, and sqlc generates type-safe code from SQL queries, **And** the project follows the defined package organization (cmd/press-out/, internal/, sql/, web/)

3. **Given** the SQLite database does not exist, **When** the application starts, **Then** the database is created at the configured DB_PATH with the lifts table schema applied via migration files, **And** configuration loads from environment variables with defaults (PORT=8080, DATA_DIR=./data, DB_PATH=./data/press-out.db)

## Tasks / Subtasks

- [ ] Initialize Go module and project skeleton (AC: 2)
  - [ ] Run `go mod init press-out`
  - [ ] Create full directory structure: `cmd/press-out/`, `internal/config/`, `internal/handler/`, `internal/pipeline/`, `internal/pipeline/stages/`, `internal/storage/`, `internal/sse/`, `internal/mediapipe/`, `internal/claude/`, `sql/schema/`, `sql/queries/`, `web/templates/layouts/`, `web/templates/pages/`, `web/templates/partials/`, `web/static/`, `data/`, `testdata/videos/`
  - [ ] Create `.gitignore` (ignore `data/`, `web/static/output.css`, `press-out` binary, `.env`, `*.db`)
  - [ ] Create `.env.example` with PORT=8080, DATA_DIR=./data, DB_PATH=./data/press-out.db, MEDIAPIPE_API_KEY=

- [ ] Create Makefile with build targets (AC: 2)
  - [ ] `build`: runs sqlc generate + tailwind build + go build
  - [ ] `sqlc-generate`: `sqlc generate`
  - [ ] `tailwind-build`: `tailwindcss -i web/static/input.css -o web/static/output.css --minify`
  - [ ] `go-build`: `go build -o press-out ./cmd/press-out`
  - [ ] `test`: `go test ./...`
  - [ ] `run`: `go run ./cmd/press-out`
  - [ ] `dev`: hot-reload target (air or similar)

- [ ] Create `tailwind.config.js` with DaisyUI custom theme (AC: 1)
  - [ ] Configure content paths to scan `web/templates/**/*.html`
  - [ ] Add DaisyUI plugin with custom theme: base (#FAFAF8), text (#2D2D2D), primary (#8BA888), secondary (#C4BFAE), neutral (#EDEDEA), info (#9BB0BA), success (#7DA67D)
  - [ ] No reds, no warnings, no error colors

- [ ] Create `web/static/input.css` with Tailwind directives (AC: 1)
  - [ ] `@tailwind base; @tailwind components; @tailwind utilities;`
  - [ ] System font stack override: `-apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif`

- [ ] Create `sqlc.yaml` configuration (AC: 2)
  - [ ] Configure for SQLite with mattn/go-sqlite3
  - [ ] Set SQL schema path: `sql/schema/`
  - [ ] Set queries path: `sql/queries/`
  - [ ] Set output to `internal/storage/sqlc/`

- [ ] Create initial SQL schema `sql/schema/001_initial.sql` (AC: 3)
  - [ ] `lifts` table: `id` INTEGER PRIMARY KEY AUTOINCREMENT, `lift_type` TEXT NOT NULL (snatch/clean/clean_and_jerk), `created_at` TEXT NOT NULL (RFC3339), `coaching_cue` TEXT, `coaching_diagnosis` TEXT
  - [ ] Use snake_case column names, plural table name

- [ ] Create SQL queries `sql/queries/lifts.sql` (AC: 3)
  - [ ] `ListLifts`: SELECT all lifts ordered by created_at DESC
  - [ ] `GetLift`: SELECT single lift by id
  - [ ] `CreateLift`: INSERT lift with lift_type and created_at
  - [ ] `DeleteLift`: DELETE lift by id

- [ ] Create `internal/config/config.go` (AC: 3)
  - [ ] Load PORT (default 8080), DATA_DIR (default ./data), DB_PATH (default ./data/press-out.db), MEDIAPIPE_API_KEY from environment variables
  - [ ] Export Config struct with typed fields
  - [ ] `func Load() Config` reads env vars and applies defaults

- [ ] Create `internal/storage/db.go` — SQLite connection and migration runner (AC: 3)
  - [ ] `func NewDB(dbPath string) (*sql.DB, error)` — opens SQLite with mattn/go-sqlite3
  - [ ] `func RunMigrations(db *sql.DB, schemaDir string) error` — reads numbered .sql files from schema dir, executes in order
  - [ ] Create data directory if it doesn't exist
  - [ ] Enable WAL mode for SQLite

- [ ] Create `internal/storage/storage.go` — file path helpers (AC: 2)
  - [ ] Constants: `FileOriginal = "original.mp4"`, `FileTrimmed = "trimmed.mp4"`, `FileCropped = "cropped.mp4"`, `FileSkeleton = "skeleton.mp4"`, `FileThumbnail = "thumbnail.jpg"`, `FileKeypoints = "keypoints.json"`
  - [ ] `func LiftDir(dataDir string, liftID int64) string` — returns `data/lifts/{id}/`
  - [ ] `func LiftFile(dataDir string, liftID int64, filename string) string` — returns full file path
  - [ ] `func CreateLiftDir(dataDir string, liftID int64) error` — creates the lift directory
  - [ ] `func RemoveLiftDir(dataDir string, liftID int64) error` — removes the entire lift directory

- [ ] Create `internal/handler/routes.go` — route registration (AC: 1)
  - [ ] Register routes on `http.ServeMux`: `GET /`, `POST /lifts`, `GET /lifts/{id}`, `DELETE /lifts/{id}`, `GET /lifts/{id}/events`
  - [ ] Register HTMX partial routes: `GET /lifts/{id}/coaching`, `GET /lifts/{id}/status`
  - [ ] Serve static files from `web/static/`

- [ ] Create `internal/handler/lift.go` — lift list handler (AC: 1)
  - [ ] `HandleListLifts` — GET / — queries all lifts, renders lift-list.html
  - [ ] Passes lift data and empty state flag to template

- [ ] Create `web/templates/layouts/base.html` — HTML shell (AC: 1)
  - [ ] HTML5 doctype, viewport meta for mobile, charset
  - [ ] Link to output.css, HTMX via CDN, DaisyUI via CDN
  - [ ] Script tag for app.js
  - [ ] `{{block "content" .}}{{end}}` placeholder
  - [ ] Apply custom DaisyUI theme attribute

- [ ] Create `web/templates/pages/lift-list.html` — lift list page (AC: 1)
  - [ ] Extends base layout
  - [ ] Page title "press-out" in text-xl semibold
  - [ ] Conditional empty state when no lifts exist
  - [ ] Upload button: full-width, h-12, sage accent (#8BA888), white text, "Upload Lift" label
  - [ ] Placeholder for lift list items (will be populated in Story 1.3)
  - [ ] Mobile-only layout: 375-430px target, p-4 padding, single column

- [ ] Create `cmd/press-out/main.go` — application entry point (AC: 1, 2, 3)
  - [ ] Load config via `config.Load()`
  - [ ] Initialize SQLite DB via `storage.NewDB()` and run migrations
  - [ ] Initialize sqlc queries
  - [ ] Register routes via `handler.RegisterRoutes()`
  - [ ] Parse templates
  - [ ] Start HTTP server on configured port with slog startup message

- [ ] Add Go dependencies (AC: 2)
  - [ ] `go get github.com/mattn/go-sqlite3`
  - [ ] Run `sqlc generate` to produce generated code

- [ ] Write tests (AC: 1, 2, 3)
  - [ ] `internal/config/config_test.go` — test env var loading and defaults
  - [ ] `internal/storage/db_test.go` — test DB creation and migration runner
  - [ ] `internal/storage/storage_test.go` — test file path helpers
  - [ ] `internal/handler/lift_test.go` — test GET / returns 200, renders template, handles empty state

## Dev Notes

- This is the FOUNDATION story. Every subsequent story depends on the project structure, build system, database, and base templates established here.
- Go 1.22+ `net/http` provides enhanced routing with method + pattern matching (e.g., `GET /lifts/{id}`) — no third-party router needed.
- mattn/go-sqlite3 requires CGo and a C compiler. Ensure `CGO_ENABLED=1` in build.
- sqlc generates type-safe Go code from SQL queries. Run `sqlc generate` after any SQL file changes. Generated code goes to `internal/storage/sqlc/`.
- Tailwind standalone CLI compiles CSS without npm/node. Download the binary for the target platform.
- DaisyUI custom theme uses data attribute: `<html data-theme="press-out">` with theme defined in tailwind.config.js.
- System font stack: no web fonts, no loading delay. Set via Tailwind config or CSS.
- Logging: use `slog.Info()` for startup, config loaded, server listening. Structured JSON format.
- No authentication, no sessions, no user model — single-user personal tool.
- File constants defined in storage package prevent path construction errors across the codebase.

### Project Structure Notes

Files to create:
- `go.mod` — Go module init
- `Makefile` — build orchestration
- `sqlc.yaml` — sqlc configuration
- `tailwind.config.js` — Tailwind + DaisyUI theme
- `.gitignore` — ignore patterns
- `.env.example` — env var template
- `cmd/press-out/main.go` — entry point
- `internal/config/config.go` — env var loading
- `internal/storage/db.go` — SQLite connection + migrations
- `internal/storage/storage.go` — file path helpers
- `internal/handler/routes.go` — route registration
- `internal/handler/lift.go` — list handler
- `sql/schema/001_initial.sql` — initial schema
- `sql/queries/lifts.sql` — lift CRUD queries
- `web/templates/layouts/base.html` — HTML shell
- `web/templates/pages/lift-list.html` — lift list page
- `web/static/input.css` — Tailwind directives
- `web/static/app.js` — empty JS file (will be populated later)

### References

- [Source: architecture.md#Starter Template Evaluation] — manual scaffold, go mod init
- [Source: architecture.md#Data Architecture] — SQLite, sqlc, migration strategy, file organization
- [Source: architecture.md#Frontend Architecture] — template organization, static assets
- [Source: architecture.md#Infrastructure & Deployment] — Makefile targets, env vars, slog logging
- [Source: architecture.md#Implementation Patterns] — naming conventions, file path construction
- [Source: architecture.md#Project Structure & Boundaries] — complete directory tree
- [Source: prd.md#Web Application Specific Requirements] — server-rendered, HTMX, no JS framework
- [Source: epics.md#Story 1.1] — acceptance criteria
- [Source: ux-design-specification.md] — UX-DR1 (theme), UX-DR2 (fonts), UX-DR3 (typography), UX-DR13 (buttons), UX-DR15 (mobile-only), UX-DR16 (spacing)

## Dev Agent Record

### Agent Model Used
### Completion Notes List
### Change Log
### File List
