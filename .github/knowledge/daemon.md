# workshopd daemon

**Scope**: Consult when working on the daemon lifecycle, state manangers
snapcraft.yaml or snap hooks or anything that reads/writes the data, common, or cache directories.

Daemon lifecycle logic lives in `internal/daemon`.

State managers are located in `internal/overlord/cmdstate`, `internal/overlord/healthstate`, `internal/overlord/hookstate`,
`internal/overlord/ifacestate`, `internal/overlord/sdkstate`, `internal/overlord/workshopstate`.

## Socket-activated daemon

`workshopd` is a **socket-activated systemd notify daemon** that deactivates
if the ensure cycle has run at least once, there are no active connections
for a period of time and no active changes running.

Implications of the socket activation that the daemon relies on:

- `workshop.bin` mounts are recreated for all existing
  workshops on every daemon start up.
- All interface connections are reloaded from `state.json` on every daemon start up (see
  `internal/overlord/ifacestate/ifacemgr.go:InterfaceManager.StartUp()`).
- Xauthority cookie is updated on every daemon startup. That allows the daemon to have
  an up to date cookie after the user's logout from an X11 session with a subsequent login.
- Anything that is not written to `state.json` or to the workshop's OR SDK's
  LXD configuration is lost on deactivation. E.g. the state's cache that the daemon uses for verbose logs, see `internal/overlord/state/state.go:State.Cache()`.

## Degraded mode

Triggered when `syscheck.CheckSystem()` fails (LXD missing/incompatible/down; out of storage space).

- Blocks all non-`GET` requests with the degraded error; `GET` still works.
- A recovery ticker re-runs `CheckSystem()` continuously and clears degraded
  mode automatically — it also detects LXD disappearing _after_ startup.

## Directory layout

Three host roots, each overridable by an env var, with snap-specific defaults:

| Role   | Var               | Default (non-snap)    | Snap value                    | Refresh behavior                             |
| ------ | ----------------- | --------------------- | ----------------------------- | -------------------------------------------- |
| Data   | `WORKSHOP_DATA`   | `/var/lib/workshop`   | `$SNAP_DATA`                  | **Per revision** (tied to installed version) |
| Common | `WORKSHOP_COMMON` | falls back to Data    | `$SNAP_COMMON/workshop`       | **Shared** across revisions                  |
| Cache  | `WORKSHOP_CACHE`  | `/var/cache/workshop` | `$SNAP_COMMON/workshop/cache` | Shared                                       |

Key distinction: **Data** is revision-scoped (`$SNAP_DATA`), **Common** survives
snap refreshes (`$SNAP_COMMON`).

## Invariants

**Security backends and interfaces are registered before `ensureBackendInit` runs.**

- **Why**: if `ensureBackendInit` runs first, `repo.Plug` returns nil and
  `reloadConnections` silently skips every connection; `backendReady` then
  latches true, so the later `StartUp()` call is a no-op.
- **Where enforced**: registration ordering in `InterfaceManager.StartUp()`,
  `internal/overlord/ifacestate/ifacemgr.go`.
- **How it breaks**: reordering registration or the syscheck - connections never
  load and no error is surfaced.
