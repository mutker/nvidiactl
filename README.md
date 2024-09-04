# nvidiactl

A command-line tool providing automatic fan speed management and dynamic power limit adjustment for NVIDIA GPUs. It can optionally be run as a systemd service.

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

### Configuration options

- `interval`: Time between updates (in seconds)
- `temperature`: Maximum allowed temperature (in Celsius)
- `fanspeed`: Maximum allowed fan speed (in percent)
- `hysteresis`: Temperature change required before adjusting fan speed (in Celsius)
- `performance`: Enable performance mode, disabling power limit adjustments (boolean)
- `monitor`: Enable monitoring mode (boolean)
- `debug`: Enable debug output (boolean)

### Example configuration file (`/etc/nvidiactl.conf`)

```toml
# Maximum allowed temperature (in degrees Celsius)
temperature = 80

# Maximum allowed fan speed (in percentage)
fanspeed = 65

# Temperature hysteresis (in degrees Celsius)
hysteresis = 2

# Performance mode (true/false)
performance = false

# Monitor mode - only monitor temperature and fan speed (true/false)
monitor = false

# Debug mode - enable debugging output (true/false)
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

## Contributing

Contributions are welcome! Please feel free to submit a pull request or issue.

## License

This project is licensed under the MIT License - see the LICENSE file for details.
