# Lumograph

Lumograph is a minimalist Go command-line tool designed to fetch time-series data from VictoriaMetrics and render it into high-quality PNG charts.

## Architecture

The project is currently a single-file Go application (`main.go`) that handles:
- **CLI Parameter Parsing**: Uses the standard `flag` package.
- **Data Acquisition**: Queries VictoriaMetrics via its PromQL-compatible `query_range` API.
- **Visualization**: Renders data using the `gonum/plot` library.

## Tech Stack

- **Language**: Go (v1.25+)
- **Plotting Library**: `gonum.org/v1/plot`
- **Data Source**: VictoriaMetrics (or any Prometheus-compatible API)

## Development Conventions

- **Simplicity**: Keep the logic focused and minimize external dependencies.
- **Color Palette**: Use the predefined `Blue` (`#1E88E5`) for primary data series.
- **Error Handling**: Use `fmt.Fprintf(os.Stderr, ...)` for error reporting and exit with non-zero codes.
- **Formatting**: Adhere to standard `go fmt` patterns.

## CLI Usage

| Flag | Description | Default |
|------|-------------|---------|
| `-endpoint` | VictoriaMetrics URL (Required) | |
| `-fetch-dashboards` | Directory containing YAML config files to fetch Grafana dashboards from | |
| `-service` | Service name for query substitution (Required) | |
| `-interval` | Interval string for query substitution | `5m` |
| `-start` | Start time (YYYY-MM-DD HH:MM:SS) | 24h ago |
| `-end` | End time (YYYY-MM-DD HH:MM:SS) | now |
| `-token` | Bearer token for auth. Can also be set via `PMM_TOKEN` environment variable. | |
| `-debug` | Print detailed HTTP request/response info | `false` |

### Configuration

The tool uses an internally embedded `graphs.json` database containing the graph configurations.

### Running the tool

```bash
# Run directly
go run main.go

# Build and run
go build -o lumograph main.go
./lumograph
```

## Project Road Map

- [ ] Modularize `main.go` into separate packages for API and Plotting.
- [ ] Add support for multiple queries in a single chart.
- [ ] Implement unit tests for data transformation logic.
