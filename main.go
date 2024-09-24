// Copyright © 2024 Mutker Telag <dark.dusk53443@fastmail.com>
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
	minTemperature        = 50 // Minimum temperature (°C) before auto fan control is activated to allow zero RPM
	powerLimitWindowSize  = 5  // Number of power limit readings to average
	maxPowerLimitChange   = 10 // Maximum power limit adjustment (in watts) per interval
	wattsPerDegree        = 5  // Power limit adjustment (in watts) per degree of temperature difference
	temperatureWindowSize = 5  // Number of temperature readings to average
	maxFanSpeedChange     = 5  // Maximum fan speed change per interval (in percentage points)
)

type Config struct {
	Interval          int
	Temperature       int
	FanSpeed          int
	Hysteresis        int
	PerformanceMode   bool
	Monitor           bool
	Debug             bool
	Verbose           bool
	MaxTemperature    int `default:"80"`
	DefaultInterval   int `default:"2"`
	MaxFanSpeed       int `default:"100"`
	DefaultHysteresis int `default:"4"`
}

var (
	config          Config
	nvmlInitialized bool
	autoFanControl  bool
	logger          *log.Logger
	sysLogger       *syslog.Writer
	cacheGPU        nvml.Device
	deviceSync      sync.Once
	gpuUUID         string
	gpuName         string
	gpuFans         int

	temperatureHistory []int
	averageTemperature int
	currentFanSpeed    int
	lastFanSpeed       int
	minFanSpeedLimit   int
	maxFanSpeedLimit   int

	powerLimitHistory []int
	averagePowerLimit int
	lastPowerLimit    int
	currentPowerLimit int
	defaultPowerLimit int
	minPowerLimit     int
	maxPowerLimit     int
)

func init() {
	log.Println("Initializing nvidiactl...")
	initLogger()
	initConfig()
}

func initLogger() error {
	// Check if running as systemd service
	isSystemd := os.Getenv("JOURNAL_STREAM") != ""

	if isSystemd {
		// When running under systemd, log to stdout without timestamps
		logger = log.New(os.Stdout, "", 0)
	} else {
		// For non-systemd environments, keep the existing logging setup
		logger = log.New(os.Stdout, "", log.LstdFlags)
	}

	// Only initialize syslog if not running under systemd and verbose logging is enabled
	if !isSystemd && config.Verbose {
		var err error
		sysLogger, err = syslog.New(syslog.LOG_INFO|syslog.LOG_DAEMON, "nvidiactl")
		if err != nil {
			return fmt.Errorf("failed to initialize syslog: %v", err)
		}
	}

	return nil
}

func initConfig() {
	flag.IntVar(&config.Interval, "interval", config.Interval, "Interval between updates")
	flag.IntVar(&config.Temperature, "temperature", config.Temperature, "Maximum allowed temperature")
	flag.IntVar(&config.FanSpeed, "fanspeed", config.FanSpeed, "Maximum allowed fan speed")
	flag.IntVar(&config.Hysteresis, "hysteresis", config.Hysteresis, "Temperature hysteresis")
	flag.BoolVar(&config.PerformanceMode, "performance", config.PerformanceMode, "Performance mode: Do not adjust power limit")
	flag.BoolVar(&config.Monitor, "monitor", config.Monitor, "Only monitor temperature and fan speed")
	flag.BoolVar(&config.Debug, "debug", config.Debug, "Enable debugging mode")
	flag.BoolVar(&config.Verbose, "verbose", config.Verbose, "Enable verbose logging")
	flag.Parse()

	viper.SetConfigName("nvidiactl.conf")
	viper.SetConfigType("toml")
	viper.AddConfigPath("/etc")
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			log.Fatalf("failed to read config file: %s", err)
		}
	}

	// Override config file values with command line flags
	flag.Visit(func(f *flag.Flag) {
		viper.Set(f.Name, f.Value.String())
	})

	// Unmarshal the configuration
	if err := viper.Unmarshal(&config); err != nil {
		log.Fatalf("failed to unmarshal config: %s", err)
	}

	// Initialize syslog if verbose logging is enabled

	if config.Verbose {
		var err error
		sysLogger, err = syslog.New(syslog.LOG_INFO|syslog.LOG_DAEMON, "nvidiactl")
		if err != nil {
			logMessage("ERROR", "failed to initialize syslog: %v", err)
			os.Exit(1)
		}
	}
}

func initNvml() error {
	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		return logMessage("ERROR", "failed to initialize NVML: %v", nvml.ErrorString(ret))
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

	// Get GPU UUID
	uuid, ret := device.GetUUID()
	if ret != nvml.SUCCESS {
		return logMessage("ERROR", "failed to get UUID of device: %v", nvml.ErrorString(ret))
	}
	gpuUUID = uuid

	// Get GPU friendly name
	var err error
	gpuName, err = getGPUName()
	if err != nil {
		return logMessage("ERROR", "failed to get name of device: %v", err)
	}

	// Get amount of GPU fans
	gpuFans, err = getGPUFans()
	if err != nil {
		return logMessage("ERROR", "failed to get amount of fans: %v", err)
	}

	// Initialize the cached device
	if _, err := getGPUHandle(); err != nil {
		return logMessage("ERROR", "failed to initialize cached device: %v", err)
	}

	// Detect fan speed limits
	if err := initFanSpeed(); err != nil {
		return err
	}

	// Detect power limits
	if err := initPowerLimits(); err != nil {
		return err
	}

	logMessage("INFO", "Successfully initialized. Starting...")
	return nil
}

func initFanSpeed() error {
	if err := getMinMaxFanSpeed(); err != nil {
		return logMessage("ERROR", "failed to get min/max fan speed: %v", err)
	}

	// Add a small delay before querying the initial fan speed
	time.Sleep(1 * time.Second)

	currentFanSpeed, err := getCurrentFanSpeed()
	if err != nil {
		logMessage("WARNING", "failed to get current fan speed: %v", err)
	}

	lastFanSpeed = currentFanSpeed
	logMessage("INFO", "Initial fan speed detected: %d%%", lastFanSpeed)
	return nil
}

func initPowerLimits() error {
	if err := getMinMaxPowerLimits(); err != nil {
		return logMessage("ERROR", "failed to get power limits: %v", err)
	}

	var err error
	currentPowerLimit, err = getCurrentPowerLimit()
	if err != nil {
		return logMessage("ERROR", "failed to get current power limit: %v", err)
	}

	logMessage("DEBUG", "Initial power limit: %dW", currentPowerLimit)
	setPowerLimit(defaultPowerLimit)
	return nil
}

func main() {
	if err := initLogger(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
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
			currentFanSpeed, _ := getCurrentFanSpeed()

			averageTemperature := updateTemperatureHistory(currentTemperature)
			targetFanSpeed := calculateFanSpeed(averageTemperature, config.Temperature, config.FanSpeed)
			averagePowerLimit := updatePowerLimitHistory(currentPowerLimit)
			targetPowerLimit := calculatePowerLimit(averageTemperature, config.Temperature, currentFanSpeed, config.FanSpeed, currentPowerLimit)

			if !config.Monitor {
				// Fan speed control logic
				if averageTemperature <= minTemperature {
					if !autoFanControl {
						enableAutoFanControl()
						autoFanControl = true
					}
				} else {
					if autoFanControl {
						logMessage("DEBUG", "Temperature (%d°C) above minimum (%d°C). Switching to manual fan control.", averageTemperature, minTemperature)
						autoFanControl = false
					}

					if targetFanSpeed != currentFanSpeed && !applyHysteresis(targetFanSpeed, currentFanSpeed, config.Hysteresis, config.PerformanceMode) {
						setFanSpeed(targetFanSpeed)
					}
				}

				// Power limit adjustment
				if !config.PerformanceMode {
					averagePowerLimit := updatePowerLimitHistory(targetPowerLimit)
					gradualPowerLimit := getGradualPowerLimit(averagePowerLimit, lastPowerLimit)

					// Ensure we respect the minimum power limit
					gradualPowerLimit = clamp(gradualPowerLimit, minPowerLimit, maxPowerLimit)

					if gradualPowerLimit != currentPowerLimit {
						setPowerLimit(gradualPowerLimit)
						lastPowerLimit = gradualPowerLimit
					}
				} else if currentPowerLimit < maxPowerLimit {
					setPowerLimit(maxPowerLimit)
					targetPowerLimit = maxPowerLimit
				}
			}

			logStatus(currentTemperature, averageTemperature, currentFanSpeed, targetFanSpeed, currentPowerLimit, targetPowerLimit, averagePowerLimit)
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
	enableAutoFanControl()
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

func applyHysteresis(newSpeed, lastSpeed, hysteresis int, performanceMode bool) bool {
	if performanceMode {
		// In performance mode, allow changes more freely
		return false
	}
	// Hysteresis logic for normal mode
	return (newSpeed >= lastSpeed-hysteresis) && (newSpeed <= lastSpeed+hysteresis)
}

// Logging functions
func logStatus(currentTemperature, averageTemperature, currentFanSpeed, targetFanSpeed, currentPowerLimit, targetPowerLimit, averagePowerLimit int) {
	if config.Debug {
		logMessage("DEBUG", "current_fan_speed=%d target_fan_speed=%d last_set_fan_speed=%d max_fan_speed=%d current_temperature=%d average_temperature=%d min_temperature=%d max_temperature=%d current_power_limit=%d target_power_limit=%d average_power_limit=%d min_power_limit=%d max_power_limit=%d min_fan_speed=%d max_fan_speed=%d hysteresis=%d monitor=%t performance=%t auto_fan_control=%t",
			currentFanSpeed,
			targetFanSpeed,
			lastFanSpeed,
			config.FanSpeed,
			currentTemperature,
			averageTemperature,
			minTemperature,
			config.Temperature,
			currentPowerLimit,
			targetPowerLimit,
			averagePowerLimit,
			minPowerLimit,
			maxPowerLimit,
			minFanSpeedLimit,
			maxFanSpeedLimit,
			config.Hysteresis,
			config.Monitor,
			config.PerformanceMode,
			autoFanControl,
		)
	} else if config.Verbose {
		logMessage("INFO", "fan_speed=%d temperature=%d (avg=%d) power_limit=%d (avg=%d)",
			currentFanSpeed, currentTemperature, averageTemperature, currentPowerLimit, averagePowerLimit)
	}
}

func logMessage(level string, format string, v ...interface{}) error {
	msg := fmt.Sprintf(format, v...)
	logMsg := fmt.Sprintf("%s: %s", level, msg)

	isSystemd := os.Getenv("JOURNAL_STREAM") != ""

	if level == "INFO" || config.Debug || (config.Verbose && level != "DEBUG") {
		if isSystemd {
			// When running under systemd, only log the message without the level prefix
			logger.Print(msg)
		} else {
			// For non-systemd environments, log with the level prefix
			logger.Print(logMsg)
		}

		// Only use syslog when not running under systemd
		if sysLogger != nil && !isSystemd {
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
func getGPUHandle() (nvml.Device, error) {
	var initErr error
	deviceSync.Do(func() {
		var ret nvml.Return
		cacheGPU, ret = nvml.DeviceGetHandleByUUID(gpuUUID)
		if ret != nvml.SUCCESS {
			initErr = logMessage("ERROR", "failed to get device handle: %v", nvml.ErrorString(ret))
		}
	})
	return cacheGPU, initErr
}

func getCurrentTemperature() (int, error) {
	device, _ := getGPUHandle()
	temp, ret := device.GetTemperature(nvml.TEMPERATURE_GPU)
	if ret != nvml.SUCCESS {
		return 0, logMessage("ERROR", "failed to get temperature handle: %v", ret)
	}
	return int(temp), nil
}

func updateTemperatureHistory(currentTemperature int) int {
	temperatureHistory = append(temperatureHistory, currentTemperature)
	if len(temperatureHistory) > temperatureWindowSize {
		temperatureHistory = temperatureHistory[1:]
	}

	sum := 0
	for _, temp := range temperatureHistory {
		sum += temp
	}
	return sum / len(temperatureHistory)
}

func getCurrentFanSpeed() (int, error) {
	device, err := getGPUHandle()
	if err != nil {
		return 0, fmt.Errorf("failed to get device handle: %v", err)
	}

	// Try to get fan speed for each fan
	for i := 0; i < gpuFans; i++ {
		fanSpeed, ret := device.GetFanSpeed_v2(i)
		if ret == nvml.SUCCESS && fanSpeed > 0 {
			return int(fanSpeed), nil
		}
		logMessage("DEBUG", "GetFanSpeed_v2 for fan %d returned: %v", i, nvml.ErrorString(ret))
	}

	// If individual fan query fails, fall back to general GetFanSpeed
	fanSpeed, ret := device.GetFanSpeed()
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("failed to get fan speed: %v", nvml.ErrorString(ret))
	}

	// if fanSpeed == 0 {
	// 	logMessage("WARNING", "GetFanSpeed returned 0, which may be incorrect")
	// }

	return int(fanSpeed), nil
}

func getGradualFanSpeed(targetFanSpeed, currentFanSpeed int) int {
	if targetFanSpeed > currentFanSpeed {
		return min(targetFanSpeed, currentFanSpeed+maxFanSpeedChange)
	} else if targetFanSpeed < currentFanSpeed {
		return max(targetFanSpeed, currentFanSpeed-maxFanSpeedChange)
	}
	return currentFanSpeed
}

func getMinMaxFanSpeed() error {
	device, _ := getGPUHandle()
	minSpeed, maxSpeed, ret := device.GetMinMaxFanSpeed()
	if ret != nvml.SUCCESS {
		return logMessage("ERROR", "failed to get min/max fan speed: %v", nvml.ErrorString(ret))
	}

	minFanSpeedLimit = int(minSpeed)
	maxFanSpeedLimit = int(maxSpeed)

	logMessage("DEBUG", "Fan speed limits detected. Min: %d%%, Max: %d%%", minFanSpeedLimit, maxFanSpeedLimit)

	return nil
}

func calculateFanSpeed(averageTemperature, maxTemperature, configMaxFanSpeed int) int {
	// If temperature is at or below minimum, return 0 to indicate no change needed
	if averageTemperature <= minTemperature {
		return 0
	}

	if averageTemperature <= minTemperature {
		return 0
	}

	if averageTemperature >= maxTemperature {
		return configMaxFanSpeed
	}

	tempRange := float64(maxTemperature - minTemperature)
	tempPercentage := float64(averageTemperature-minTemperature) / tempRange

	// Use a more gradual curve for fan speed increase
	fanSpeedPercentage := math.Pow(tempPercentage, 1.5)
	fanSpeedRange := configMaxFanSpeed - minFanSpeedLimit
	targetFanSpeed := clamp(int(float64(fanSpeedRange)*fanSpeedPercentage)+minFanSpeedLimit, minFanSpeedLimit, configMaxFanSpeed)

	return targetFanSpeed
}

func getGPUName() (string, error) {
	device, err := getGPUHandle()
	if err != nil {
		return "", err
	}

	name, ret := device.GetName()
	if ret != nvml.SUCCESS {
		return "", logMessage("ERROR", "failed to get GPU name: %v", nvml.ErrorString(ret))
	}

	logMessage("INFO", "Detected GPU: %s", name)
	return name, nil
}

func getGPUFans() (int, error) {
	device, err := getGPUHandle()
	if err != nil {
		return 0, err
	}

	count, ret := device.GetNumFans()
	if ret != nvml.SUCCESS {
		return 0, logMessage("ERROR", "failed to get number of fans: %v", nvml.ErrorString(ret))
	}

	logMessage("INFO", "Detected %d fans", count)
	return int(count), nil
}

func setFanSpeed(fanSpeed int) {
	device, _ := getGPUHandle()

	for i := 0; i < gpuFans; i++ {
		ret := nvml.DeviceSetFanSpeed_v2(device, i, fanSpeed)
		if ret != nvml.SUCCESS {
			logMessage("INFO", "failed to set fan speed for fan%d: %v", i, nvml.ErrorString(ret))
		}
	}

	currentFanSpeed, _ := getCurrentFanSpeed()
	lastFanSpeed = currentFanSpeed
	logMessage("DEBUG", "Setting fan speed to %d%%", lastFanSpeed)
}

func calculateFanSpeedPercentage(tempPercentage float64) float64 {
	if config.PerformanceMode {
		return math.Pow(tempPercentage, 1.5)
	}
	return math.Pow(tempPercentage, 2)
}

func calculateTargetFanSpeed(fanSpeedPercentage float64, maxFanSpeed int) int {
	fanSpeedRange := maxFanSpeed - minFanSpeedLimit
	targetFanSpeed := int(float64(fanSpeedRange)*fanSpeedPercentage) + minFanSpeedLimit
	return clamp(targetFanSpeed, minFanSpeedLimit, maxFanSpeed)
}

func enableAutoFanControl() {
	device, _ := getGPUHandle()
	ret := nvml.DeviceSetDefaultFanSpeed_v2(device, 0)
	if ret != nvml.SUCCESS {
		logMessage("ERROR", "failed to set default fan speed: %v", nvml.ErrorString(ret))
	}
	logMessage("DEBUG", "Temperature (%d°C) at or below minimum (%d°C). Enabling auto fan control.", averageTemperature, minTemperature)
}

func getCurrentPowerLimit() (int, error) {
	device, err := getGPUHandle()
	if err != nil {
		return 0, logMessage("ERROR", "failed to get device handle: %v", err)
	}

	powerLimit, ret := device.GetPowerManagementLimit()
	if ret != nvml.SUCCESS {
		return 0, logMessage("ERROR", "failed to get current power limit: %v", nvml.ErrorString(ret))
	}

	return int(powerLimit / 1000), nil // Convert milliwatts to watts
}

func getMinMaxPowerLimits() error {
	device, _ := getGPUHandle()
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

	return nil
}

func calculatePowerLimit(currentTemperature, targetTemperature, currentFanSpeed, maxFanSpeed, currentPowerLimit int) int {
	if config.PerformanceMode {
		return maxPowerLimit
	}

	if currentTemperature > targetTemperature && currentFanSpeed >= maxFanSpeed {
		tempDiff := currentTemperature - targetTemperature
		adjustment := min(tempDiff*wattsPerDegree, maxPowerLimitChange)
		newPowerLimit := currentPowerLimit - adjustment
		return clamp(newPowerLimit, minPowerLimit, maxPowerLimit)
	}

	if currentTemperature <= targetTemperature && currentPowerLimit < maxPowerLimit {
		return min(currentPowerLimit+wattsPerDegree, maxPowerLimit)
	}

	return currentPowerLimit
}

func updatePowerLimitHistory(newPowerLimit int) int {
	powerLimitHistory = append(powerLimitHistory, newPowerLimit)
	if len(powerLimitHistory) > powerLimitWindowSize {
		powerLimitHistory = powerLimitHistory[1:]
	}

	sum := 0
	for _, limit := range powerLimitHistory {
		sum += limit
	}
	return sum / len(powerLimitHistory)
}

func setPowerLimit(powerLimit int) {
	device, _ := getGPUHandle()
	ret := device.SetPowerManagementLimit(uint32(powerLimit * 1000)) // Convert watts to milliwatts
	if ret != nvml.SUCCESS {
		logMessage("ERROR", "Failed to set power limit to %dW: %v", powerLimit, nvml.ErrorString(ret))
	} else {
		currentPowerLimit = powerLimit
		lastPowerLimit = powerLimit
		logMessage("DEBUG", "Successfully set power limit to %dW", powerLimit)
	}
}

func getGradualPowerLimit(targetPowerLimit, currentPowerLimit int) int {
	if targetPowerLimit > currentPowerLimit {
		return min(targetPowerLimit, currentPowerLimit+maxPowerLimitChange)
	} else if targetPowerLimit < currentPowerLimit {
		return max(targetPowerLimit, currentPowerLimit-maxPowerLimitChange)
	}
	return currentPowerLimit
}

func resetPowerLimit() {
	setPowerLimit(defaultPowerLimit)
}
