# Commands

Run all commands from the `sigil/` root.

## Primary Make Targets

- `make help`: show the supported local targets.
- `make build`: build the `./sigil` executable.
- `make fmt`: format tracked Go files with `gofmt`.
- `make tidy`: reconcile `go.mod` and `go.sum`.
- `make test-unit`: run unit-focused tests for `./cmd/...` and `./internal/...`.
- `make test-acceptance`: run Godog-backed acceptance coverage in `./acceptance/...`.
- `make test`: run the full Go test suite uncached with `-count=1`.
- `make verify`: run `fmt`, `tidy`, and the full test suite.
- `make clean`: clear the Go test cache and remove the local `./sigil` binary.
- `make clean-run`: delete the local `./.sigil` runtime-artifact directory.
- `make clean-all`: run `clean` and `clean-run` together.

## Direct Go Entry Points

- `go test ./... -count=1`: full uncached verification when you need raw `go test` output.
- `go test ./acceptance/... -count=1`: acceptance-only verification during behavior work.
- `go test ./cmd/... ./internal/... -count=1`: fast unit-focused verification during implementation.
- `go run . --help`: inspect the root CLI without building a binary first.

## Config and Example Files

- Default application config path: `./sigil.yaml`
- Default run config path: `./sigil-run.yaml`
- Example templated run config: `./examples/templated/sigil-run.yaml`

Prefer the Make targets unless you need narrower raw `go test` or `go run` output.
