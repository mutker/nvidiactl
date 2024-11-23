# nvidiactl

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/codeberg.org/mutker/nvidiactl)](https://goreportcard.com/report/codeberg.org/mutker/nvidiactl)

nvidiactl is a command-line tool providing automatic fan speed management and dynamic power limit adjustment for NVIDIA GPUs. It can optionally be run as a systemd service.

## Features

- 🌡️ **Automatic fan speed control** based on GPU temperature
- ⚡ **Dynamic power limit management** to balance performance and noise
- 🎛️ **Customizable temperature thresholds** and fan speed limits
- 🔁 **Hysteresis support** to prevent rapid fluctuations in fan speed
- 🚀 **Performance mode** to prioritize GPU performance over noise reduction
- 📊 **Monitoring mode** for observing GPU stats without making changes
- 📝 **Logging support** for syslog and systemd journal
- 🔧 **Direct GPU interaction** using NVML (NVIDIA Management Library)
- 🖥️ **Standalone application** or systemd service functionality
- 🔍 **Debug mode** for detailed logging and troubleshooting
- 📈 **Telemetry collection** in local database for advanced statistics

## Installation

### Arch Linux (AUR)

Install the `nvidiactl-git` package from the AUR using your preferred AUR helper. For example, using `yay`:

   ```
   yay -S nvidiactl-git
   ```

After installation, you can enable and start the systemd service with `sudo systemctl enable --now nvidiactl.service`, and if you want to enable verbose or debug logging, add an override with `sudo systemctl edit nvidiactl`:

```
[Service]
ExecStart=
ExecStart=/usr/bin/nvidiactl --verbose # or --debug
```

### Building from Source

1. Ensure you have Go 1.23 or later installed on your system.

2. Clone the repository:
   ```
   git clone https://codeberg.org/mutker/nvidiactl.git
   cd nvidiactl
   ```

3. Build the application: `go build -v -o nvidiactl ./cmd/nvidiactl`

4. Copy the binary to a location in your PATH: `sudo cp nvidiactl /usr/local/bin/`

5. Copy the example configuration file, and edit as needed: `sudo cp nvidiactl.example.conf /etc/nvidiactl.conf`

## Configuration

Configuration is done via a TOML file at `/etc/nvidiactl.conf` or through command-line arguments. Command-line arguments take precedence over the config file.

```toml
# Time between updates (in seconds, default: 2)
interval = 2

# Maximum allowed temperature (in Celsius, default: 80)
temperature = 80

# Maximum allowed fan speed (in percent, default: 100)
fanspeed = 100

# Temperature change required before adjusting fan speed (in Celsius, default: 4)
hysteresis = 4

# Enable performance mode: disables power limit adjustments (boolean, default: false)
performance = false

# Enable monitor mode: only monitor temperature and fan speed (boolean, default: false)
monitor = false

# Log level: debug, info, warning, error (string, default: "info")
log_level = "info"

# Enable telemetry collection (boolean, default: false)
telemetry = false

# Path to the telemetry database file (string, default: "/var/lib/nvidiactl/telemetry.db")
database = "/var/lib/nvidiactl/telemetry.db"
```

## Usage

Simply call `nvidiactl` after configuring `/etc/nvidiactl.conf`, or via the command-line, e.g. `nvidiactl --temperature=85 --fanspeed=80 --performance`. Optional telemetry collection in a local SQLite3 database (default: `/var/lib/nvidiactl/telemetry.db`) can be enabled with `--telemetry`.

Enable monitoring mode ("dry run", only prints statistics with no changes to fan speeds or power limits): `nvidiactl --monitor`

## Building

Ensure you have Go 1.23 or later installed, and then run:

```
go build -v -o nvidiactl ./cmd/nvidiactl
```

## Roadmap

- Add presets for fan and power limit adjustment curves that can be applied during runtime
- Enhance telemetry collection with additional metrics: frequency, memory usage, processes, etc.
- Add objective-based (perf vs power vs noise) fan and power limit adjustment curves utilizing collected statistics

## Architecture

This project follows Domain-Driven Design principles:
- Clear domain boundaries (gpu, telemetry)
- Interface-driven design
- Rich domain-specific error handling
- Dependency injection
- Clear separation of concerns

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed design documentation.

## Contributing

Contributions are welcome! Please feel free to submit a pull request or issue.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
