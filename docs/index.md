<div align="center">
  <img src="../docs/assets/img/lumograph.png" alt="Lumograph Logo" width="300"/>
</div>

# Lumograph

**Lumograph** is a vibe-coded replacement for the Grafana renderer plugin when used with 'Percona Monitoring and Management', and Dipper.
It is a single, statically compiled Go binary with embedded fonts and configurations to render timeseries data as PNG images.

---

## Requirements

- Generic Linux x86_64, or macos arm64
- Accessible Percona Monitoring and Management server
-- Must create a new, temporary, Service Account with 'Admin' level privilege ([PMM-15192](https://perconadev.atlassian.net/browse/PMM-15192))
- For Dipper sync, Lumograph needs internet access to Dipper

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
- `-endpoint` : The base URL of the PMM server (e.g., `https://pmm1.int.company.com`).
- `-groups` : A comma-separated list of graph groups to render (e.g., `mysql,innodb,os`). See `list-groups` command below.
- `-service` : The name of the service for the graphs. See `list-services` command below.
- `-token` : PMM API token. *(Can also set the `PMM_TOKEN` environment variable).*

**Optional Flags:**
- `-cluster-name` : Required for cluster-based graphs (ie: PXC, Mongo, etc)
- `-database` : Filter for PostgreSQL databases
- `-debug` : Enables verbose structured logging, including raw HTTP payloads and data structures.
- `-end` : Absolute end time (`YYYY-MM-DD HH:MM:SS`). Defaults to now.
- `-insecure-tls` : Disable TLS certificate verification (for self-signed certs)
- `-interval` : The time interval for rates/averages. Defaults to `5m` (PMM default).
- `-node` : Must supply the node name if not automatically detected.
- `-outdir` : Output directory for graphs. Defaults to service name.
- `-replset` : MongoDB replica set name
- `-start` : Absolute start time (`YYYY-MM-DD HH:MM:SS`). Defaults to 24 hours ago.


**Example:**
```bash
$ export PMM_TOKEN="your_secure_token"
$ ./lumograph get-graphs -endpoint https://pmmdemo.percona.com -service percona-mongo-0-rs1 -groups os,wiredtiger
```

---

### `list-services`
*Lists all available services from the PMM inventory API.*

This command fetches the remote PMM inventory, and returns a list of known services.

**Required Flags:**
- `-endpoint` : The PMM URL.
- `-token` : The PMM API token (or `PMM_TOKEN` env var).

**Optional Flags:**
- `-debug` : Enables verbose structured logging, including raw HTTP payloads and data structures.
- `-insecure-tls` : Disable TLS certificate verification (for self-signed certs)

**Example:**
```bash
$ export PMM_TOKEN="your_secure_token"
$ ./lumograph list-services -endpoint https://pmmdemo.percona.com
```

---

### `list-groups`
*Lists all available graph groups found in the Lumograph configuration.*

This command outputs a list of the various graph group tags available for rendering. Use these names in the `-groups` flag of the `get-graphs` command.

**Example:**
```bash
$ ./lumograph list-groups
```

---

### `dipper-sync`
*Compresses the images in <image-directory>, and uploads them to Dipper.*

**Required Flags:**
- `-hostname` : Hostname associated with the images.
- `-projectid` : Dipper Project ID.
- `-token` : Dipper API token (can also use DIPPER_TOKEN env var).

**Example:**
```bash
$ export DIPPER_TOKEN="dipper_token"
$ ./lumograph dipper-sync -hostname mysql1.int.company.com -projectid PS0018888 mysql1-service/
```

---

## Development / Contributing

[golangci-lint](https://golangci-lint.run/) is used to enforce certain code styles, and to perform additional lint checks.

Clone the repo, install the required mods, and build.

```bash
-- Get mods
$ go mod tidy

-- Generate graph configs
$ go generate

-- Build lumograph
$ go fmt && go vet && golangci-lint run && go build -o lumograph .
```

### Adding additional graphs

The various graphs, and graph groups that Lumograph fetches are managed through standard YAML files in the `resources/graphs/` directory.
Modify the YAML file by adding the graph name to one of the existing lists. You may need to view the JSON source in PMM to find the graph title.
Open a pull request with the new graph titles added.

During the build process, the `go generate` command reads those YAML files, downloads the raw Grafana PMM dashboard JSON definitions, and transforms the data into `lumographs.go` native structs.
```
