// Copyright © 2024 Mutker Telag <witty.text5011@fastmail.com>
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/syslog"
	"math"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/spf13/viper"
)

const (
	minTemperature          = 60
	maxPowerLimitAdjustment = 20
	wattPerDegree           = 5
	defaultMaxTemperature   = 80
	defaultInterval         = 2
	defaultMaxFanSpeed      = 100
	defaultHysteresis       = 2
)

var (
	config            Config
	nvmlInitialized   bool
	logger            *log.Logger
	sysLogger         *syslog.Writer
	cacheDevice       nvml.Device
	cacheFanSpeeds    bool
	cachePowerLimits  bool
	deviceSync        sync.Once
	gpuUUID           string
	lastFanSpeed      int
	minFanSpeedLimit  int
	maxFanSpeedLimit  int
	currentPowerLimit int
	defaultPowerLimit int
	minPowerLimit     int
	maxPowerLimit     int
)

type Config struct {
	Interval        int
	Temperature     int
	FanSpeed        int
	Hysteresis      int
	PerformanceMode bool
	Monitor         bool
	Debug           bool
	Verbose         bool
}

func init() {
	initLogger()
	initConfig()
}

func initLogger() error {
	logger = log.New(os.Stdout, "", log.LstdFlags)

	if config.Verbose {
		var err error
		sysLogger, err = syslog.New(syslog.LOG_INFO|syslog.LOG_DAEMON, "nvidiactl")
		if err != nil {
			return fmt.Errorf("failed to initialize syslog: %v", err)
		}
	}

	return nil
}

func initConfig() {
	flag.IntVar(&config.Temperature, "temperature", defaultMaxTemperature, "Maximum allowed temperature")
	flag.IntVar(&config.Interval, "interval", defaultInterval, "Interval between updates")
	flag.IntVar(&config.FanSpeed, "fanspeed", defaultMaxFanSpeed, "Maximum allowed fan speed")
	flag.IntVar(&config.Hysteresis, "hysteresis", defaultHysteresis, "Temperature hysteresis")
	flag.BoolVar(&config.PerformanceMode, "performance", false, "Performance mode: Do not adjust power limit")
	flag.BoolVar(&config.Monitor, "monitor", false, "Only monitor temperature and fan speed")
	flag.BoolVar(&config.Debug, "debug", false, "Enable debugging mode")
	flag.BoolVar(&config.Verbose, "verbose", false, "Enable verbose logging")
	flag.Parse()

	viper.SetConfigName("nvidiactl.conf")
	viper.SetConfigType("toml")
	viper.AddConfigPath("/etc")
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			log.Fatalf("Error reading config file: %s", err)
		}
	}

	flag.Visit(func(f *flag.Flag) {
		viper.Set(f.Name, f.Value.String())
	})

	if err := viper.Unmarshal(&config); err != nil {
		log.Fatalf("Error unmarshaling config: %s", err)
	}

	if config.Verbose {
		var err error
		sysLogger, err = syslog.New(syslog.LOG_INFO|syslog.LOG_DAEMON, "nvidiactl")
		if err != nil {
			logMessage("ERROR", "Failed to initialize syslog: %v", err)
			os.Exit(1)
		}
	}
}

func initNvml() error {
	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		return logMessage("ERROR", "Failed to initialize NVML: %v", nvml.ErrorString(ret))
	}
	nvmlInitialized = true
	defer func() {
		if !nvmlInitialized {
			nvml.Shutdown()
		}
	}()

	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return logMessage("ERROR", "failed to get device count: %v", nvml.ErrorString(ret))
	}

	if count == 0 {
		return logMessage("ERROR", "failed to find any NVIDIA GPUs")
	}

	// We'll use the first GPU (index 0)
	device, ret := nvml.DeviceGetHandleByIndex(0)
	if ret != nvml.SUCCESS {
		return logMessage("ERROR", "failed to get device at index 0: %v", nvml.ErrorString(ret))
	}

	uuid, ret := device.GetUUID()
	if ret != nvml.SUCCESS {
		return logMessage("ERROR", "failed to get UUID of device: %v", nvml.ErrorString(ret))
	}

	name, ret := device.GetName()
	if ret != nvml.SUCCESS {
		return logMessage("ERROR", "failed to get name of device: %v", nvml.ErrorString(ret))
	}

	gpuUUID = uuid
	logMessage("INFO", "Detected GPU: %s", name)

	// Initialize the cached device
	if _, err := getDeviceHandle(); err != nil {
		return logMessage("ERROR", "failed to initialize cached device: %w", err)
	}

	if err := initializeFanSpeed(); err != nil {
		return err
	}

	if err := initializePowerLimits(); err != nil {
		return err
	}

	return nil
}

func initializeFanSpeed() error {
	if err := getMinMaxFanSpeed(); err != nil {
		return logMessage("ERROR", "failed to get min/max fan speed: %w", err)
	}

	// Add a small delay before querying the initial fan speed
	time.Sleep(500 * time.Millisecond)

	currentFanSpeed, err := getCurrentFanSpeed()
	if err != nil {
		return logMessage("ERROR", "failed to get current fan speed: %w", err)
	}

	// Verify the fan speed
	if currentFanSpeed == minFanSpeedLimit {
		// Double-check after a short delay
		time.Sleep(500 * time.Millisecond)
		currentFanSpeed, err = getCurrentFanSpeed()
		if err != nil {
			return logMessage("ERROR", "failed to get current fan speed on second attempt: %w", err)
		}
	}

	lastFanSpeed = currentFanSpeed

	logMessage("DEBUG", "Initial fan speed: %d%%", lastFanSpeed)
	return nil
}

func initializePowerLimits() error {
	if err := getMinMaxPowerLimits(); err != nil {
		return logMessage("ERROR", "failed to get power limits: %w", err)
	}

	var err error
	currentPowerLimit, err = getCurrentPowerLimit()
	if err != nil {
		return logMessage("ERROR", "failed to get current power limit: %w", err)
	}
	cachePowerLimits = true

	logMessage("DEBUG", "Initial power limit: %dW", currentPowerLimit)
	return nil
}

func main() {
	if err := initLogger(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	if err := initNvml(); err != nil {
		logMessage("ERROR", "Initialization failed: %v", err)
		os.Exit(1)
	}
	defer nvml.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go handleSignals(cancel)

	loop(ctx)

	cleanup()
}

func loop(ctx context.Context) {
	ticker := time.NewTicker(time.Second * time.Duration(config.Interval))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentTemperature, _ := getCurrentTemperature()
			targetFanSpeed := calculateFanSpeed(currentTemperature, config.Temperature, config.FanSpeed)
			targetPowerLimit := calculatePowerLimit(currentTemperature, config.Temperature)

			if !config.Monitor {
				newFanSpeed := applyHysteresis(targetFanSpeed, lastFanSpeed, config.Hysteresis)
				if newFanSpeed != lastFanSpeed {
					setFanSpeed(newFanSpeed)
					lastFanSpeed = newFanSpeed
				}
				newPowerLimit := applyHysteresis(targetPowerLimit, currentPowerLimit, config.Hysteresis)
				if newPowerLimit != currentPowerLimit {
					setPowerLimit(newPowerLimit)
				}
			}

			logStatus(currentTemperature, lastFanSpeed, currentPowerLimit)
		}
	}
}

func handleSignals(cancel context.CancelFunc) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	logMessage("INFO", "Received termination signal.")
	cancel()
}

func cleanup() {
	setPowerLimit(defaultPowerLimit)
	logMessage("INFO", "Exiting...")
	if sysLogger != nil {
		sysLogger.Close()
	}
}

// Helper functions
func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Logging functions
func logStatus(temperature, fanSpeed, powerLimit int) {
	if config.Debug {
		actualFanSpeed, _ := getCurrentFanSpeed()
		logMessage("DEBUG", "Temperature: current=%d°C, max=%d°C", temperature, config.Temperature)
		logMessage("DEBUG", "Fan Speed: current=%d%%, target=%d%%, last=%d%%, max=%d%%",
			actualFanSpeed, fanSpeed, lastFanSpeed, config.FanSpeed)
		logMessage("DEBUG", "Power Limit: current=%dW, min=%dW, max=%dW",
			powerLimit, minPowerLimit, maxPowerLimit)
		logMessage("DEBUG", "Fan Limits: min=%d%%, max=%d%%", minFanSpeedLimit, maxFanSpeedLimit)
		logMessage("DEBUG", "Settings: hysteresis=%d, monitor=%v, performance=%v",
			config.Hysteresis, config.Monitor, config.PerformanceMode)
	} else if config.Verbose {
		logMessage("INFO", "Temperature: %d°C, Fan Speed: %d%%, Power Limit: %dW",
			temperature, fanSpeed, powerLimit)
	}
}

func logMessage(level string, format string, v ...interface{}) error {
	msg := fmt.Sprintf(format, v...)
	logMsg := fmt.Sprintf("%s: %s", level, msg)

	if config.Debug || (config.Verbose && level != "DEBUG") {
		logger.Print(logMsg)
		if sysLogger != nil {
			switch level {
			case "DEBUG":
				sysLogger.Debug(msg)
			case "INFO":
				sysLogger.Info(msg)
			case "WARNING":
				sysLogger.Warning(msg)
			case "ERROR":
				sysLogger.Err(msg)
			}
		}
	}

	if level == "ERROR" {
		return fmt.Errorf(msg)
	}
	return nil
}

// GPU functions
func getDeviceHandle() (nvml.Device, error) {
	var initErr error
	deviceSync.Do(func() {
		var ret nvml.Return
		cacheDevice, ret = nvml.DeviceGetHandleByUUID(gpuUUID)
		if ret != nvml.SUCCESS {
			initErr = logMessage("ERROR", "failed to get device handle: %v", nvml.ErrorString(ret))
		}
	})
	return cacheDevice, initErr
}

func getCurrentTemperature() (int, error) {
	device, _ := getDeviceHandle()
	temp, ret := device.GetTemperature(nvml.TEMPERATURE_GPU)
	if ret != nvml.SUCCESS {
		return 0, logMessage("ERROR", "failed to get temperature handle: %w", ret)
	}
	return int(temp), nil
}

func getCurrentFanSpeed() (int, error) {
	device, err := getDeviceHandle()
	if err != nil {
		return 0, logMessage("ERROR", "failed to get device handle: %w", err)
	}

	fanSpeed, ret := device.GetFanSpeed()
	if ret != nvml.SUCCESS {
		return 0, logMessage("ERROR", "failed to get fan speed: %v", nvml.ErrorString(ret))
	}

	return int(fanSpeed), nil
}

func getMinMaxFanSpeed() error {
	// Return cached values if available
	if cacheFanSpeeds {
		return nil
	}

	device, _ := getDeviceHandle()
	minSpeed, maxSpeed, ret := device.GetMinMaxFanSpeed()
	if ret != nvml.SUCCESS {
		return logMessage("ERROR", "failed to get min/max fan speed: %v", nvml.ErrorString(ret))
	}

	minFanSpeedLimit = int(minSpeed)
	maxFanSpeedLimit = int(maxSpeed)

	// Cache the results
	cacheFanSpeeds = true

	logMessage("DEBUG", "Fan speed limits detected. Min: %d%%, Max: %d%%", minFanSpeedLimit, maxFanSpeedLimit)

	return nil
}

func calculateFanSpeed(currentTemperature, maxTemperature, maxFanSpeed int) int {
	if currentTemperature <= minTemperature {
		return minFanSpeedLimit
	}
	if currentTemperature >= maxTemperature {
		return maxFanSpeed
	}

	tempRange := float64(maxTemperature - minTemperature)
	tempPercentage := float64(currentTemperature-minTemperature) / tempRange

	var fanSpeedPercentage float64
	if config.PerformanceMode {
		// More aggressive curve for performance mode
		fanSpeedPercentage = math.Pow(tempPercentage, 1.5)
	} else {
		// Original curve for normal mode
		fanSpeedPercentage = math.Pow(tempPercentage, 2)
	}

	targetFanSpeed := int(float64(maxFanSpeed-minFanSpeedLimit)*fanSpeedPercentage) + minFanSpeedLimit
	return clamp(targetFanSpeed, minFanSpeedLimit, maxFanSpeed)
}

func setFanSpeed(fanSpeed int) {
	device, _ := getDeviceHandle()

	logMessage("DEBUG", "Setting fan speed to %d%%", fanSpeed)

	ret := nvml.DeviceSetFanSpeed_v2(device, 0, fanSpeed) // Assuming 0 is the first fan
	if ret != nvml.SUCCESS {
		logMessage("INFO", "Error setting fan speed: %v", nvml.ErrorString(ret))
	}

	currentFanSpeed, _ := getCurrentFanSpeed()
	logMessage("DEBUG", "Current fan speed after setting: %d%%", currentFanSpeed)
	lastFanSpeed = currentFanSpeed
}

func applyHysteresis(newValue, currentValue, hysteresis int) int {
	if abs(newValue-currentValue) <= hysteresis {
		return currentValue
	}
	return newValue
}

func setLastFanSpeed() {
	currentSpeed, _ := getCurrentFanSpeed()
	lastFanSpeed = currentSpeed
	logMessage("DEBUG", "Updated lastFanSpeed: %d%%", lastFanSpeed)
}

func enableAutoFanControl() {
	device, _ := getDeviceHandle()

	ret := nvml.DeviceSetDefaultFanSpeed_v2(device, 0)
	if ret != nvml.SUCCESS {
		logMessage("INFO", "Error setting default fan speed: %v", nvml.ErrorString(ret))
	}
}

func getCurrentPowerLimit() (int, error) {
	device, err := getDeviceHandle()
	if err != nil {
		return 0, logMessage("ERROR", "failed to get device handle: %w", err)
	}

	powerLimit, ret := device.GetPowerManagementLimit()
	if ret != nvml.SUCCESS {
		return 0, logMessage("ERROR", "failed to get current power limit: %v", nvml.ErrorString(ret))
	}

	return int(powerLimit / 1000), nil // Convert milliwatts to watts
}

func getMinMaxPowerLimits() error {
	// Return cached values if available
	if cachePowerLimits {
		return nil
	}

	device, _ := getDeviceHandle()
	minLimit, maxLimit, ret := device.GetPowerManagementLimitConstraints()
	if ret != nvml.SUCCESS {
		return logMessage("ERROR", "failed to get power management limit constraints: %v", nvml.ErrorString(ret))
	}

	defaultLimit, ret := device.GetPowerManagementDefaultLimit()
	if ret != nvml.SUCCESS {
		return logMessage("ERROR", "failed to get default power management limit: %v", nvml.ErrorString(ret))
	}

	minPowerLimit = int(minLimit / 1000) // Convert from milliwatts to watts
	maxPowerLimit = int(maxLimit / 1000)
	defaultPowerLimit = int(defaultLimit / 1000)

	logMessage("DEBUG", "Power limits detected. Min: %dW, Max: %dW, Default: %dW", minPowerLimit, maxPowerLimit, defaultPowerLimit)

	// Cache the results
	cachePowerLimits = true

	return nil
}

func calculatePowerLimit(currentTemperature, targetTemperature int) int {
	if config.PerformanceMode {
		return maxPowerLimit
	}

	tempDiff := currentTemperature - targetTemperature
	adjustment := tempDiff * wattPerDegree
	newPowerLimit := currentPowerLimit - adjustment

	return clamp(newPowerLimit, minPowerLimit, maxPowerLimit)
}

func setPowerLimit(powerLimit int) {
	if powerLimit == currentPowerLimit {
		return
	}

	device, _ := getDeviceHandle()
	ret := device.SetPowerManagementLimit(uint32(powerLimit * 1000)) // Convert watts to milliwatts
	if ret != nvml.SUCCESS {
		logMessage("INFO", "Error setting power limit: %v", nvml.ErrorString(ret))
		return
	}

	logMessage("INFO", "Setting power limit to %dW", powerLimit)
	logMessage("DEBUG", "Power limit set successfully from %dW to %dW", currentPowerLimit, powerLimit)
	currentPowerLimit = powerLimit
}

func resetPowerLimit() {
	setPowerLimit(defaultPowerLimit)
}
