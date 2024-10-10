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

## Configuration

Configuration can be done via a TOML file located at `/etc/nvidiactl.conf`, or through command-line arguments. Command-line arguments take precedence over the config file.

```toml
# Time between updates (in seconds, default: 2)
interval = 2

# Maximum allowed temperature (in Celsius, default: 80)
temperature = 80

# Maximum allowed fan speed (in percent, default: 100)
fanspeed = 100

# Temperature change required before adjusting fan speed (in Celsius, default: 4)
hysteresis = 4

# Enable performance mode - disables power limit adjustments (boolean, default: false)
performance = false

# Enable monitor mode - only monitor temperature and fan speed (boolean, default: false)
monitor = false

# Enable debug mode - output detailed logging (boolean, default: false)
debug = false

# Enable verbose logging (boolean, default: false)
verbose = false
```

## Usage

Run with default configuration (or configured by `/etc/nvidiactl.conf`):

```
nvidiactl
```

Run with custom settings:

```
nvidiactl --temperature=85 --fanspeed=80 --performance
```

Enable monitoring mode (only prints statistics, with no change to fan speeds or power limits):

```
nvidiactl --monitor
```

### Run a Systemd Service

Copy `nvidiactl.server` to `/usr/lib/systemd/system/nvidiactl.service`, then enable and start the service with `sudo systemctl enable --now nvidiactl`

```
sudo cp nvidiactl.service /usr/lib/systemd/system/nvidiactl.service
sudo systemctl daemon-reload
sudo systemctl enable --now nvidiactl.service
```

If you want to enable verbose or debug logging, add an overide with `sudo systemctl edit nvidiactl`:

```
[Unit]
ExecStart=
ExecStart=/usr/bin/nvidiactl --verbose
```

Remember to reload systemd (`sudo systemctl daemon-reload`) and restart the service (`systemctl restart nvidiactl`).

## Building

Ensure you have Go installed, and then run:

```
go build -o build
```

## Roadmap

- Add presets for fan and power limit adjustment curves that can be applied during runtime
- Add detailed statistics collector and storage for further analysis
- Add objective-based (perf vs power vs noise) fan and power limit adjustment curves utilizing statistics

## Contributing

Contributions are welcome! Please feel free to submit a pull request or issue.

## License

This project is licensed under the MIT License - see the LICENSE file for details.
