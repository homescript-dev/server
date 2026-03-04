# AGENTS.md

## Scope
- This file applies to the whole repository: `/Users/roman/devel/homescript-server`.
- Goal: help coding agents make safe, minimal, verifiable changes.

## Project Basics
- Language: Go.
- Main binary: `homescript-server`.
- Entry point: `cmd/server/main.go`.
- Core modules live in `internal/`:
- `mqtt`, `events`, `executor`, `scheduler`, `devices`, `storage`, `discovery`.

## Build And Run
- Build: `go build -o homescript-server ./cmd/server`
- Quick test: `go test ./...`
- Make equivalents:
- `make build`
- `make test`
- `make discover`
- `make run`

## Configuration And Runtime Paths
- Default config root: `./config`
- Device config: `./config/devices/devices.yaml`
- HA discovery cache: `./config/devices/ha_configs.json`
- Default DB: `./data/state.db`

## Change Guidelines
- Prefer small, targeted patches; avoid large refactors unless requested.
- Preserve existing event/script layout under `config/events`.
- Do not commit generated binaries (`homescript-server`, `homescript/server`) unless explicitly asked.
- Keep logging style consistent with `internal/logger`.
- Avoid introducing new dependencies without clear need.

## Validation Checklist
- Run `go test ./...` after changes.
- If touching scheduling, MQTT routing, or Lua execution paths, validate startup path:
- `./homescript-server run --config ./config --db ./data/state.db --mqtt-broker tcp://localhost:1883`
- If touching discovery/config generation:
- `./homescript-server discover --config ./config --mqtt-broker tcp://localhost:1883`

## Known Gaps
- Automated tests are currently minimal (`[no test files]` in most packages).
- Prefer adding focused unit tests when fixing logic/concurrency issues.
