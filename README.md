<div align="center">
  <img src="resources/lumograph.png" alt="Lumograph Logo" width="300"/>
</div>

# Lumograph

**Lumograph** is a vibe-coded replacement for the Grafana renderer plugin when used with 'Percona Monitoring and Management', and Dipper.
It is a single, statically compiled Go binary with embedded fonts and configurations to render timeseries data as PNG images.

---

## Installation

Download a precompiled release binary to the target machine.

---

## Usage & Commands

Lumograph uses a series of subcommands. You must provide one of the following commands to execute the tool:

### `get-graphs`
*Generates the charts specified by the 'groups' flag, by querying the provided PMM endpoint.*

This is the primary functionality of Lumograph. It fetches the data from the remote PMM server, and renders the graph images to the local disk.

**Required Flags:**
- `-endpoint` : The base URL of PMM server (e.g., `https://pmmdemo.percona.com`).
- `-token` : PMM API token. *(Can also set the `PMM_TOKEN` environment variable).*
- `-service` : The name of the service for the graphs. See `list-services` command below.
- `-groups` : A comma-separated list of graph groups to render (e.g., `mysql,innodb,os`). See `list-groups` command below.

**Optional Flags:**
- `-node` : Must supply the node name if not automatically detected.
- `-interval` : The time interval for rates/averages. Defaults to `5m` (PMM default).
- `-start` : Absolute start time (`YYYY-MM-DD HH:MM:SS`). Defaults to 24 hours ago.
- `-end` : Absolute end time (`YYYY-MM-DD HH:MM:SS`). Defaults to now.
- `-debug` : Enables verbose structured logging, including raw HTTP payloads and data structures.

**Example:**
```bash
export PMM_TOKEN="your_secure_token"
./lumograph get-graphs -endpoint https://pmmdemo.percona.com -service percona-mongo-0-rs1 -groups os,wiredtiger
```

---

### `list-services`
*Lists all available services from the PMM inventory API.*

This command fetches the remote PMM inventory and returns a list of known services.

**Required Flags:**
- `-endpoint` : The PMM URL.
- `-token` : The PMM API token (or `PMM_TOKEN` env var).

**Example:**
```bash
export PMM_TOKEN="your_secure_token"
./lumograph list-services -endpoint https://pmmdemo.percona.com
```

---

### `list-groups`
*Lists all available graph groups found in the lumograph configuration.*

This command outputs a list of the various graph group tags available for rendering. Use these names in the `-groups` flag of the `get-graphs` command.

**Example:**
```bash
./lumograph list-groups
```

---

### `rebuild-config`
*Fetches and rebuilds the internal JSON configuration from local YAML files.*

The various graphs, and graph groups that Lumograph fetches are managed through standard YAML files in the `graphs/` directory.
This command reads those YAML files, downloads the raw Grafana PMM dashboard JSON definitions, and transforms the data to match Lumograph's config.
The final config file is embedded within the Lumograph binary.

*NOTE:* This command is only used in the development process, and only required when one of the YAML definitions changes. If you wish to add, or change the graphs, edit the YAML file, rebuild-config, then recompile lumograph.

**Usage:**
```bash
# Rebuild the standard suite by pointing to a directory
./lumograph rebuild-config graphs/

# Rebuild lumograph after rebuilding the config
go build -o lumograph .
```

---

## Development / Contributing

[golangci-lint](https://golangci-lint.run/) is used to enforce certain code styles, and to perform additional lint checks.

Clone the repo, install the required mods, and build.

```bash
$ go mod tidy
$ go fmt && go vet && golangci-lint run && go build -o lumograph .

-- Fetch config, and rebuild
$ ./lumograph rebuild-config
$ go build -o lumograph .
```
