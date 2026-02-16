# Node.js SDK for Workshop

A development environment for Node.js projects. It provides a Node.js runtime
built from source, manages package manager caches via persistent mounts, and
exposes the V8 inspector for remote debugging.

---

## Overall design of the SDK

This SDK:

- Provides Node.js runtime (v20.x) built from source with Corepack enabled,
  making `pnpm` and `yarn` available alongside `node` and `npm`.
- Adds Node.js binaries to `PATH` and generates shell completions for
  `node` and `npm`.
- Sets the `npm` global prefix to `~/.local` for package installation without
  `sudo`.
- Configures persistent mount plugs for `npm`, `pnpm`, and `yarn` caches
  to preserve package downloads across workshop updates.
- Provides a consistent Node.js version across all supported bases.

---

## Reference workshop

A minimal workshop:

```yaml
# workshop.yaml
name: node-app
base: ubuntu@24.04
sdks:
  - name: node
    channel: 24.04/edge

actions:
  install: |
    npm install
  test: |
    npm test
```

This demonstrates a basic Node.js workflow with persistent package caches.

---

## Using the SDK

### Prerequisites, project layout

1. No prerequisite SDKs are required.
2. Your Node.js project (with a `package.json`) should be in your project
   directory:

   ```bash
   git clone <YOUR_REPO_URL>
   ```

3. On launch, the SDK configures `PATH`, shell completions, and the `npm`
   global prefix. No package installation happens automatically; you control
   when to run `npm install` or equivalent via workshop actions or otherwise.

### Install dependencies and build

Once the workshop is ready:

```bash
workshop shell
npm install
npm run build
```

The `npm` download cache is mapped from your host via the `npm-cache` mount plug
to the `~/.npm/_cacache` directory inside the workshop.

Subsequent `npm install` runs are faster because they reuse cached packages
from the persistent mount.

To see where the packages are stored on the host:

```bash
workshop info
```

### Test and run

Launch test and run commands from within the workshop, for example:

```bash
workshop shell
npm test
npm start
```

Use `npx` to run locally installed binaries without global installation.

### Using pnpm or yarn

Corepack is enabled, so `pnpm` and `yarn` are available without additional
setup:

```bash
workshop shell
pnpm install
# or
yarn install
```

By default, `pnpm` uses `/project/.pnpm-store` as its content-addressable
store. To use the persistent `pnpm-store` mount instead (which may be slower):

```bash
pnpm config set storeDir ~/.local/share/pnpm/store
```

### Remote debugging

The SDK exposes the V8 inspector via a tunnel slot on port 9229. To debug a
Node.js application remotely, start it with `--inspect` inside the workshop:

```bash
node --inspect your-app.js
```

Then connect the tunnel in Workshop and attach your debugger (for example,
Chrome DevTools or VS Code) to the forwarded port on the host.

---

## Installed components

When this SDK is installed in a workshop, it provides:

| Component | Description |
|-----------|-------------|
| `node` | Node.js JavaScript runtime (v20.x) |
| `npm` | Node.js package manager |
| `corepack` | Node.js tool for managing package manager versions |
| `pnpm` | Fast, disk space efficient package manager (via Corepack shim) |
| `yarn` | Alternative package manager (via Corepack shim) |
| `npx` | Executes binaries from `npm` packages (included with `npm`) |

The Node.js version is determined by the SDK release, not the base image.
Both Ubuntu 22.04 and Ubuntu 24.04 receive the same Node.js version.
Instead of updating the version manually, always use `workshop refresh`.

Note: Build tools such as `gcc` and `make` are not included. If your project
has native dependencies that need compilation, install the toolchain separately.

---

## Platforms

| Base | Architecture | Status |
|------|--------------|--------|
| `ubuntu@24.04` | `amd64` | Supported |
| `ubuntu@22.04` | `amd64` | Supported |

## Channels

Currently, only `edge` channels are available:

| Channel | Description |
|---------|-------------|
| `24.04/edge` | Latest development builds for Ubuntu 24.04 |
| `22.04/edge` | Latest development builds for Ubuntu 22.04 |

---

## Plugs (resources this SDK consumes)

### `npm-cache`

- Interface: `mount`
- Workshop target: `/home/workshop/.npm/_cacache`
- Purpose: Persists the `npm` download cache between workshop updates.

### `pnpm-cache`

- Interface: `mount`
- Workshop target: `/home/workshop/.cache/pnpm`
- Purpose: Persists `pnpm` metadata and cache files between workshop updates.

### `pnpm-store`

- Interface: `mount`
- Workshop target: `/home/workshop/.local/share/pnpm/store`
- Purpose: Provides a persistent content-addressable store for `pnpm`.

### `yarn-cache`

- Interface: `mount`
- Workshop target: `/home/workshop/.cache/yarn`
- Purpose: Persists Yarn's download cache between workshop updates.

## Slots (resources this SDK provides)

### `inspector`

- Interface: `tunnel`
- Endpoint: `9229`
- Purpose: Exposes the
  [Node.js V8 inspector](https://nodejs.org/api/debugger.html#v8-inspector-integration-for-nodejs)
  port for remote debugging on the host.

---

## Documentation and guidance

- [Node.js official documentation](https://nodejs.org/docs/latest-v20.x/api/)
- [npm documentation](https://docs.npmjs.com/)
- [Corepack documentation](https://nodejs.org/api/corepack.html)
- [pnpm documentation](https://pnpm.io/)
- [Yarn documentation](https://yarnpkg.com/)
- [Workshop documentation](https://canonical-workshop.readthedocs-hosted.com/latest/)

---

## Community and support

- Node.js community: [Node.js Discussions](https://github.com/nodejs/node/discussions)
- Workshop forum:
  [Workshop Discourse](https://discourse.canonical.com/c/engineering/sdk/34)
- Please review our
  [Code of Conduct](https://ubuntu.com/community/ethos/code-of-conduct) before
  participating.

---

## Contributions

All contributions, including code, documentation updates, and issue reports,
are welcome!

- See `CONTRIBUTING.md` for guidelines.
- Open issues or pull requests on the official repository.

---

## License and copyright

Copyright 2025 Canonical Ltd.

This SDK is licensed under the
[MIT License](https://opensource.org/licenses/MIT), the same license as
[Node.js](https://github.com/nodejs/node/blob/main/LICENSE).
