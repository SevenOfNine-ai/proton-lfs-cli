# Repository Guidelines

## Project Structure & Module Organization

- `cmd/adapter/`: Go Git LFS custom transfer adapter (`main.go`, backend/client/pass-cli helpers).
- `tests/integration/`: End-to-end style integration tests for Git LFS behavior, concurrency, timeout, and SDK flows.
- `proton-lfs-bridge/`: Node.js workspace service that fronts SDK operations; includes unit tests in `proton-lfs-bridge/tests/`.
- `docs/`: Current architecture, testing, project state, and operations docs (start at `docs/README.md`).
- `submodules/`: Upstream dependencies (`git-lfs`, `pass-cli`, `proton-drive-cli`) used for reference and integration.

## Build, Test, and Development Commands

- `make setup`: Prepare `.env` and install Go + JS dependencies.
- `make build`: Build adapter binary to `bin/git-lfs-proton-adapter`.
- `make test`: Run core adapter tests.
- `make test-sdk`: Run Node/Jest LFS bridge tests for `proton-lfs-bridge`.
- `make test-integration`: Run Go integration tests (`-tags integration`).
- `make check-sdk-prereqs && make test-integration-sdk`: Validate/pass-cli-driven SDK integration path.
- `make build-drive-cli`: Build the proton-drive-cli TypeScript bridge.
- `make test-integration-proton-drive-cli`: Run proton-drive-cli integration tests.
- `make test-integration-credentials`: Run credential flow security tests.
- `make fmt && make lint`: Run formatting and lint checks before pushing.

## Coding Style & Naming Conventions

- Go: run `go fmt`/`go vet`; keep code idiomatic and error handling explicit.
- JavaScript: use ESLint + Prettier config in `proton-lfs-bridge/`.
- Tests: Go files use `*_test.go`; Node tests use `*.test.js`.
- Keep names descriptive and scoped by concern (`backend.go`, `client.go`, `session.js`, `fileManager.js`).
- Prefer small, composable functions over large handlers.

## Testing Guidelines

- Add unit tests with every behavior change in both Go and Node layers.
- Add/extend integration tests in `tests/integration/` for protocol or workflow changes.
- Target meaningful coverage on new code paths (project guidance: ~80%+ for new logic).
- For SDK backend checks, set prerequisites first (`pass-cli login`, optional `SDK_BACKEND_MODE=proton-drive-cli`, and external `PROTON_LFS_BRIDGE_URL` when needed).
- The `SDK_BACKEND_MODE=real` value is accepted as a legacy alias for `proton-drive-cli`.

## Commit & Pull Request Guidelines

- Git history is minimal (`Initial commit`), so enforce consistency moving forward.
- Use concise imperative commits, preferably Conventional Commit style (example: `feat(adapter): handle init retry`).
- Keep PRs focused; include:
- What changed and why.
- Test evidence (`make test`, `make test-integration-sdk`, etc.).
- Docs updates when behavior/config changed.
- Never commit secrets; use Proton Pass references and `.env.example` patterns.
