# Repository Guidelines

## Project Structure & Module Organization

- `cmd/adapter/`: Go Git LFS custom transfer adapter (`main.go`, backend/client/pass-cli helpers).
- `tests/integration/`: End-to-end style integration tests for Git LFS behavior, concurrency, timeout, and SDK flows.
- `proton-sdk-service/`: Node.js workspace service that fronts SDK operations; includes unit tests in `proton-sdk-service/tests/`.
- `docs/`: Current architecture, testing, project state, and operations docs (start at `docs/README.md`).
- `submodules/`: Upstream dependencies (`git-lfs`, `sdk`, `pass-cli`) used for reference and integration.

## Build, Test, and Development Commands

- `make setup`: Prepare `.env` and install Go + JS dependencies.
- `make build`: Build adapter binary to `bin/git-lfs-proton-adapter`.
- `make test`: Run core adapter tests.
- `make test-sdk`: Run Node/Jest tests for `proton-sdk-service`.
- `make test-integration`: Run Go integration tests (`-tags integration`).
- `make check-sdk-prereqs && make test-integration-sdk`: Validate/pass-cli-driven SDK integration path.
- `make fmt && make lint`: Run formatting and lint checks before pushing.

## Coding Style & Naming Conventions

- Go: run `go fmt`/`go vet`; keep code idiomatic and error handling explicit.
- JavaScript: use ESLint + Prettier config in `proton-sdk-service/`.
- Tests: Go files use `*_test.go`; Node tests use `*.test.js`.
- Keep names descriptive and scoped by concern (`backend.go`, `client.go`, `session.js`, `fileManager.js`).
- Prefer small, composable functions over large handlers.

## Testing Guidelines

- Add unit tests with every behavior change in both Go and Node layers.
- Add/extend integration tests in `tests/integration/` for protocol or workflow changes.
- Target meaningful coverage on new code paths (project guidance: ~80%+ for new logic).
- For SDK real-mode checks, set prerequisites first (`pass-cli login`, optional `SDK_BACKEND_MODE=real`, and external `PROTON_SDK_SERVICE_URL` when needed).

## Commit & Pull Request Guidelines

- Git history is minimal (`Initial commit`), so enforce consistency moving forward.
- Use concise imperative commits, preferably Conventional Commit style (example: `feat(adapter): handle init retry`).
- Keep PRs focused; include:
- What changed and why.
- Test evidence (`make test`, `make test-integration-sdk`, etc.).
- Docs updates when behavior/config changed.
- Never commit secrets; use Proton Pass references and `.env.example` patterns.
