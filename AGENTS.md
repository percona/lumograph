# Lumograph

Lumograph is a Go command-line tool that fetches time-series data from a Percona
Monitoring and Management (PMM) / VictoriaMetrics endpoint and renders it into
high-quality PNG charts. It can also list available services and graph groups,
and package/upload rendered charts to Dipper.

## Tech Stack

- **Language**: Go (see `go.mod`, currently 1.25+)
- **Logging**: `go.uber.org/zap` (structured, colored dev logger)
- **Plotting**: `gonum.org/v1/plot`
- **Fonts**: Inter (Medium/Bold) TTFs embedded via `//go:embed` (`resources/fonts/`)
- **Config generation**: `gopkg.in/yaml.v3`
- **Data source**: VictoriaMetrics / any Prometheus-compatible `query_range` API

## Architecture

Single `main` package split across focused files (not a monolith):

- `main.go` — entry point; dispatches to a subcommand's `execute*` function.
- `config.go` — `LumoConfig`, flag sets, flag/env/positional parsing, token & time resolution.
- `errors.go` — all sentinel (static) errors, grouped by concern.
- `logger.go` — `zap` logger setup (debug toggles level/caller/stacktrace).
- `httpclient.go` — shared `httpClient`; `-insecure-tls` disables TLS verification.
- `services.go` — PMM inventory API (list services, look up a service by name).
- `get_graphs.go` — `get-graphs` orchestration, flag validation, embedded fonts, output paths.
- `graph_fetch.go` — builds and executes the VictoriaMetrics `query_range` request.
- `graph_render.go` — coordinates fetching + plotting into a PNG.
- `graph_plot.go` — plot primitives (palette, grids, tickers, series parsing).
- `graph_legend.go` — value formatting and the legend table.
- `dipper.go` — `dipper-sync`: tar.gz the PNGs and upload them to Dipper.
- `utils.go` — value formatting, snake_case, PromQL variable interpolation, config validation.
- `types.go` — `VMResponse`, `GraphConfig`, `SeriesConfig`, `TableRow`.
- `rebuild-config.go` — `//go:build ignore` generator (run via `go generate`).
- `lumographs.go` — **generated** (`// DO NOT EDIT`); `LumoGraphs map[string][]GraphConfig`.

## Subcommands

Run `lumograph <command> -h` for the full flag list.

- **`get-graphs`** — renders charts for a service. Key flags: `-endpoint`, `-service`
  (required), `-groups` (comma-separated, required), plus optional `-node`,
  `-cluster-name`, `-database`, `-replset`, `-outdir`, `-interval` (default `5m`),
  `-start`, `-end`, `-token`, `-debug`, `-insecure-tls`.
- **`list-groups`** — prints available graph groups to stdout.
- **`list-services`** — lists PMM services (grouped by type, sorted) to stdout.
  Flags: `-endpoint`, `-token`, `-debug`, `-insecure-tls`.
- **`dipper-sync`** — compresses a directory of PNGs into a `.tar.gz` and uploads it
  to Dipper via multipart POST (`X-Dipper-Auth` header, `project_id` + `hostname`
  fields). Flags: `-token`, `-projectid`, `-hostname` (all required) and a single
  positional argument: the image directory.

### Tokens & environment variables

`-token` may be supplied via environment variable instead of the flag; providing
both is an error:
- `get-graphs` / `list-services`: `PMM_TOKEN`
- `dipper-sync`: `DIPPER_TOKEN`

## Graph configuration & code generation

`lumographs.go` is generated from the YAML files in `resources/graphs/`
(`os`, `mysql`, `pgsql`, `mongo`, `valkey`). Each YAML dashboard declares a
`groups:` list and the graph titles to pull; the generator downloads the pinned
upstream PMM Grafana dashboard JSON, extracts each graph's PromQL, and writes the
native Go structs. A graph may belong to multiple groups and is emitted under each
group key in `LumoGraphs`.

Regenerate after editing YAML (requires network to reach GitHub):

```bash
go generate ./...
gofmt -w lumographs.go
```

PromQL expressions use placeholders interpolated at query time (see
`interpolateGraphConfig` in `utils.go`): `$service_name`, `$ns_service_name`,
`$interval`, `$node_name`, `$cluster`, `$replication_set`, `$set`, `$rs_nm`,
`$database`.

## Development Conventions

- **Errors**: define static sentinel errors in `errors.go`; wrap with `%w`
  (`fmt.Errorf("%w: %w", ErrX, err)`). No dynamic `errors.New`/`fmt.Errorf`
  without a wrapped sentinel (enforced by `err113`).
- **Fatal exits**: return errors up the stack; let the top-level `execute*` /
  `main` path call `zap.S().Fatal*`. Avoid fatal calls deep in the stack.
- **Logging vs. output**: diagnostics/logs go to **stderr** via `zap`; machine-
  readable command output (e.g. `list-*`) goes to **stdout** so it can be piped.
- **HTTP**: use the shared `httpClient`, always with a context timeout and
  `http.NewRequestWithContext`.
- **Formatting**: `gofmt` / `goimports` clean.
- **Linting**: must pass `golangci-lint run ./...` with zero issues (config in
  `.golangci.yaml`; note `cyclop` max complexity 15 and `gocritic` enable-all).
- **Simplicity**: keep files focused; prefer idiomatic, minimal-dependency code.

## Common Commands

```bash
go build -o lumograph .        # build
golangci-lint run ./...        # lint (must be clean)
go generate ./...              # regenerate lumographs.go from upstream PMM
```
