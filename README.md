# nvidiactl

A tool providing dynamic fan speed and power limit adjustments for NVIDIA GPUs, balancing performance and noise. It can optionally be run as a systemd service.

## Features

- Automatic fan speed control based on GPU temperature
- Dynamic power limit management to balance performance and noise
- Customizable temperature thresholds and fan speed limits
- Hysteresis to prevent rapid fluctuations in fan speed
- Performance mode to prioritize GPU performance over noise reduction
- Monitoring mode for observing GPU stats without making changes
- Direct interaction with NVIDIA GPUs using NVML (NVIDIA Management Library)

## Configuration

Configuration can be done via a TOML file located at `/etc/nvidiactl.conf`, or through command-line arguments. Command-line arguments take precedence over the config file.

```toml
# Time between updates (in seconds, default: 5)
interval = 2

# Maximum allowed temperature (in Celsius, default: 80)
temperature = 80

# Maximum allowed fan speed (in percent, default: 65)
fanspeed = 65

# Temperature change required before adjusting fan speed (in Celsius, default: 2)
hysteresis = 2

# Enable aggressive fan speed curve and disable power limit adjustments (boolean, default: false)
performance = false

# Enable monitor mode - only monitor temperature and fan speed (boolean, default: false)
monitor = false

# Enable debug mode - output detailed logging (boolean, default: false)
debug = false

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
