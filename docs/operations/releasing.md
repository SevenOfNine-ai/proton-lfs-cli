# Releasing

This project has three independently versioned artifacts:

| Artifact | Tag pattern | Publish target | Workflow |
| --- | --- | --- | --- |
| Go adapter binary | `v*` (e.g. `v0.2.0`) | GitHub Releases | `.github/workflows/build.yml` |
| LFS bridge npm package | `bridge-v*` (e.g. `bridge-v0.2.0`) | npm (`@sevenofnine-ai/proton-lfs-bridge`) | `.github/workflows/npm-publish.yml` |
| Proton Drive CLI npm package | `drive-cli-v*` (e.g. `drive-cli-v0.1.0`) | npm (`@sevenofnine-ai/proton-drive-cli`) | `.github/workflows/npm-publish.yml` |

## Go Adapter

The build workflow cross-compiles for five targets (linux/darwin amd64+arm64, windows amd64), uploads them as artifacts, and — on `v*` tags — creates a GitHub Release with all binaries attached.

### Steps

```bash
# 1. Ensure tests pass
make test && make test-integration

# 2. Tag the release
git tag v0.2.0
git push origin v0.2.0
```

The `build.yml` workflow runs automatically. When the tag matches `v*`, the `release` job creates a GitHub Release with the binaries.

### Verifying

Check the Actions tab or:

```bash
gh release view v0.2.0
```

Users install via the install script:

```bash
curl -fsSL https://raw.githubusercontent.com/SevenOfNine-ai/proton-git-lfs/main/scripts/install-adapter.sh | bash
```

Or pin a version:

```bash
VERSION=v0.2.0 curl -fsSL .../scripts/install-adapter.sh | bash
```

## LFS Bridge (npm)

The npm workflow publishes to the public npm registry using Trusted Publishing (OIDC). No `NPM_TOKEN` secret is needed — authentication is handled via GitHub's OIDC identity provider configured on npmjs.com.

### Prerequisites (one-time)

Trusted Publishing must be configured on npmjs.com:

1. Go to npmjs.com → `@sevenofnine-ai/proton-lfs-bridge` → **Settings** → **Publishing access**
2. Add trusted publisher:
   - **Repository owner**: `SevenOfNine-ai`
   - **Repository name**: `proton-git-lfs`
   - **Workflow filename**: `npm-publish.yml`
   - **Environment name**: *(blank)*

### Steps

```bash
# 1. Ensure tests pass
make test-sdk

# 2. Bump version in proton-lfs-bridge/package.json
cd proton-lfs-bridge
npm version patch   # or minor / major / 0.2.0

# 3. Commit the version bump
cd ..
git add proton-lfs-bridge/package.json
git commit -m "bridge: bump to v0.2.0"

# 4. Tag and push
git tag bridge-v0.2.0
git push origin main --tags
```

The `npm-publish.yml` workflow publishes automatically when the `bridge-v*` tag is pushed.

### Verifying

```bash
npm view @sevenofnine-ai/proton-lfs-bridge version
```

Users install via:

```bash
npm install -g @sevenofnine-ai/proton-lfs-bridge
```

## Proton Drive CLI (npm)

Published as `@sevenofnine-ai/proton-drive-cli`. This package provides:
- CLI tool (`proton-drive`) for standalone Proton Drive operations
- Shared bridge exports (`@sevenofnine-ai/proton-drive-cli/bridge`) used by proton-lfs-bridge

### Prerequisites (one-time)

Trusted Publishing must be configured on npmjs.com:

1. Go to npmjs.com → `@sevenofnine-ai/proton-drive-cli` → **Settings** → **Publishing access**
2. Add trusted publisher:
   - **Repository owner**: `SevenOfNine-ai`
   - **Repository name**: `proton-git-lfs`
   - **Workflow filename**: `npm-publish.yml`
   - **Environment name**: *(blank)*

### Steps

```bash
# 1. Ensure tests pass
cd submodules/proton-drive-cli
npx jest

# 2. Bump version in submodules/proton-drive-cli/package.json
npm version patch   # or minor / major / 0.1.1

# 3. Commit the version bump
cd ../..
git add submodules/proton-drive-cli/package.json
git commit -m "drive-cli: bump to v0.1.1"

# 4. Tag and push
git tag drive-cli-v0.1.1
git push origin main --tags
```

The `npm-publish.yml` workflow publishes automatically when the `drive-cli-v*` tag is pushed. The workflow checks out submodules, installs deps, builds TypeScript, runs tests, then publishes.

### Verifying

```bash
npm view @sevenofnine-ai/proton-drive-cli version
```

Users install via:

```bash
# As a CLI tool
npm install -g @sevenofnine-ai/proton-drive-cli

# As a library (for bridge imports)
npm install @sevenofnine-ai/proton-drive-cli
```

## Version Numbering

All three artifacts follow semver independently. A Go adapter release does not require a bridge or drive-cli release and vice versa. When multiple artifacts change in the same PR, create all relevant tags:

```bash
git tag v0.3.0
git tag bridge-v0.3.0
git tag drive-cli-v0.1.1
git push origin main --tags
```

## Manual npm Publish (fallback)

If Trusted Publishing fails or for the initial publish of a new package:

```bash
npm login

# LFS Bridge
cd proton-lfs-bridge
npm publish --access public

# Drive CLI
cd submodules/proton-drive-cli
npm run build
npm publish --access public
```

`--access public` is required on the first publish of a scoped package.
