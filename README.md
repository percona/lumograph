<div align="center">
  <img src="resources/lumograph.png" alt="Lumograph Logo" width="300"/>
  <h1>Lumograph</h1>
  <p><i>A minimalist, native Go CLI tool for generating high-quality PNG charts directly from Prometheus and VictoriaMetrics data.</i></p>
</div>

---

## 📌 Overview

**Lumograph** is designed to bypass bloated visualization dashboards and directly render beautiful, programmatic area charts from your time-series databases. Built primarily to interface with the Percona Monitoring and Management (PMM) API, it executes PromQL queries defined in a local JSON configuration and outputs sleek, presentation-ready PNGs styled with the Poppins font.

## 🚀 Features
- **Stateless & Native**: A single, statically compiled Go binary with embedded fonts and configurations.
- **Dynamic Variable Interpolation**: Seamlessly swap `$service_name`, `$node_name`, and `$interval` directly from the CLI at execution time.
- **Multi-Series Plotting**: Automatically loops through complex queries, rendering overlapping semi-transparent area charts with an intelligent color palette.
- **Auto-Calculated Visuals**: Features dynamic Y-axis scaling, custom number formatting (K/M rounding), and integrated statistical tables (Min/Max/Avg).

---

## ⚙️ Installation

Build the binary directly from source:
```bash
go build -o lumograph *.go
```

---

## 🛠️ Usage & Commands

Lumograph relies on a strict Subcommand architecture. You must provide one of the following commands to execute the tool.

### `get-graphs`
*Generates charts by querying the metrics endpoint based on the loaded configuration.*

This is the primary engine of Lumograph. It reads the local (or embedded) `graphs.json` database, interpolates your variables into the PromQL expressions, fetches the data, and saves the `.png` charts to your disk.

**Required Flags:**
- `-endpoint` : The URL of your VictoriaMetrics or PMM server (e.g., `https://pmmdemo.percona.com`).
- `-token` : Your Bearer authentication token. *(Alternatively, set the `PMM_TOKEN` environment variable to keep your shell history clean).*
- `-service` : The exact name of the service to query. This replaces `$service_name` in your queries.
- `-groups` : A comma-separated list of graph groups to render (e.g., `mysql,innodb,os`).

**Optional Flags:**
- `-node` : The Node name, which replaces `$node_name` in queries.
- `-interval` : The time interval for rates/averages, replacing `$interval`. Defaults to `5m`.
- `-start` : Absolute start time (`YYYY-MM-DD HH:MM:SS`). Defaults to 24 hours ago.
- `-end` : Absolute end time (`YYYY-MM-DD HH:MM:SS`). Defaults to now.
- `-debug` : Enables verbose structured logging, including raw HTTP payloads and data structures.

**Example:**
```bash
export PMM_TOKEN="your_secure_token"
./lumograph get-graphs -endpoint https://pmmdemo.percona.com -service percona-server-80-0-mysql -groups wiredtiger
```

---

### `list-services`
*Lists all available services from the PMM inventory API.*

If you don't know the exact string required for the `-service` flag, this command reaches into the PMM inventory endpoint and dumps a clean list of every registered service and its associated technology type.

**Required Flags:**
- `-endpoint` : The PMM URL.
- `-token` : The PMM API token (or `PMM_TOKEN` env var).

**Example:**
```bash
./lumograph list-services -endpoint https://pmmdemo.percona.com
```

---

### `list-groups`
*Lists all available graph groups found in the current JSON configuration.*

Reads the `graphs.json` file and outputs a deduplicated list of every group tag available for rendering. Use these names in the `-groups` flag of the `get-graphs` command.

**Example:**
```bash
./lumograph list-groups
```

---

### `rebuild-config`
*Fetches and rebuilds the JSON configuration from local YAML files.*

Lumograph's configuration is managed through standard YAML files (e.g., `mysql.yaml`, `mongo.yaml`). This command reads those YAML files, contacts the official Percona GitHub repository to download the raw Grafana dashboard JSONs, aggressively filters and transforms the data to match Lumograph's optimized schema, and saves the aggregated result to `graphs.json`.

**Usage:**
```bash
# Rebuild the standard suite by pointing to a directory
./lumograph rebuild-config graphs/

# Rebuild a single specific file
./lumograph rebuild-config graphs/mysql.yaml
```

---

## 🎨 Log Output
Lumograph uses `go.uber.org/zap` for high-performance, structured logging.
- By default, output is restricted to `INFO` level events (like "Saved chart to..."). 
- Pass the `-debug` flag to any command to unlock verbose, colorized tracking of the application's internal state.