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
	"math"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/spf13/viper"
)

type Config struct {
	Interval        int
	Temperature     int
	FanSpeed        int
	Hysteresis      int
	PerformanceMode bool
	Monitor         bool
	Debug           bool
}

const (
	minTemp                 = 45  // Set fans to 0% below this temperature
	maxPowerLimitAdjustment = 20  // Maximum power adjustment per cycle
	wattPerDegree           = 2   // Watts to adjust per degree of temperature difference
	defaultMaxTemp          = 80  // Default maximum temperature
	defaultInterval         = 2   // Default interval between updates
	defaultMaxFanSpeed      = 100 // Default maximum fan speed
	defaultHysteresis       = 2   // Default temperature hysteresis
)

var (
	nvmlInitialized   bool
	config            Config
	cacheDevice       nvml.Device
	cacheFanSpeeds    bool
	cachePowerLimits  bool
	deviceSync        sync.Once
	gpuUUID           string
	logger            *log.Logger
	lastFanSpeed      int
	minFanSpeedLimit  int
	maxFanSpeedLimit  int
	currentPowerLimit int
	defaultPowerLimit int
	minPowerLimit     int
	maxPowerLimit     int
)

func init() {
	log.Println("Initializing nvidiactl...")
	flags := log.LstdFlags
	if isRunningUnderSystemd() {
		flags = 0 // Remove all flags to exclude timestamps
	}
	logger = log.New(os.Stdout, "", flags)

	flag.IntVar(&config.Temperature, "temperature", defaultMaxTemp, "Maximum allowed temperature")
	flag.IntVar(&config.Interval, "interval", defaultInterval, "Interval between updates")
	flag.IntVar(&config.FanSpeed, "fanspeed", defaultMaxFanSpeed, "Maximum allowed fan speed")
	flag.IntVar(&config.Hysteresis, "hysteresis", defaultHysteresis, "Temperature hysteresis")
	flag.BoolVar(&config.PerformanceMode, "performance", false, "Performance mode: Do not adjust power limit to keep temperature below maximum")
	flag.BoolVar(&config.Monitor, "monitor", false, "Only monitor temperature and fan speed")
	flag.BoolVar(&config.Debug, "debug", false, "Enable debugging mode")
	flag.Parse()

	viper.SetConfigName("nvidiactl.conf")
	viper.SetConfigType("toml")
	viper.AddConfigPath("/etc")
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Println("Config file not found, using defaults.")
		} else {
			log.Fatalf("Error reading config file: %s", err)
		}
	} else {
		logger.Printf("Using config file: %s", viper.ConfigFileUsed())
	}

	// Command-line args take precedence
	flag.Visit(func(f *flag.Flag) {
		viper.Set(f.Name, f.Value.String())
	})

	// Ensure config struct is updated with Viper values
	if err := viper.Unmarshal(&config); err != nil {
		log.Fatalf("Error unmarshaling config: %s", err)
	}

	v := reflect.ValueOf(config)
	t := v.Type()
	if config.Debug {
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			value := v.Field(i).Interface()

			if field.Name == "Monitor" || field.Name == "Debug" {
				if boolValue, ok := value.(bool); ok && boolValue {
					logger.Printf("	%s: %v", field.Name, value)
				}
			} else if field.Name == "PerformanceMode" {
				logger.Printf("	%s: %v", "Performance mode", value)
			} else if field.Name == "FanSpeed" {
				logger.Printf("	%s: %v", "Fan speed", value)
			} else if field.Name != "LogFile" {
				logger.Printf("	%s: %v", field.Name, value)
			}
		}
	}
}

func initNvml() error {
	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to initialize NVML: %v", nvml.ErrorString(ret))
	}
	nvmlInitialized = true
	defer func() {
		if !nvmlInitialized {
			nvml.Shutdown()
		}
	}()

	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to get device count: %v", nvml.ErrorString(ret))
	}

	if count == 0 {
		return fmt.Errorf("failed to find any NVIDIA GPUs")
	}

	// We'll use the first GPU (index 0)
	device, ret := nvml.DeviceGetHandleByIndex(0)
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to get device at index 0: %v", nvml.ErrorString(ret))
	}

	uuid, ret := device.GetUUID()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to get UUID of device: %v", nvml.ErrorString(ret))
	}

	name, ret := device.GetName()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to get name of device: %v", nvml.ErrorString(ret))
	}

	gpuUUID = uuid
	logger.Printf("Detected GPU: %s\n", name)

	// Initialize the cached device
	if _, err := getDeviceHandle(); err != nil {
		return fmt.Errorf("failed to initialize cached device: %w", err)
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
		return fmt.Errorf("failed to get min/max fan speed: %w", err)
	}

	// Add a small delay before querying the initial fan speed
	time.Sleep(500 * time.Millisecond)

	currentFanSpeed, err := getCurrentFanSpeed()
	if err != nil {
		return fmt.Errorf("failed to get current fan speed: %w", err)
	}

	// Verify the fan speed
	if currentFanSpeed == minFanSpeedLimit {
		// Double-check after a short delay
		time.Sleep(500 * time.Millisecond)
		currentFanSpeed, err = getCurrentFanSpeed()
		if err != nil {
			return fmt.Errorf("failed to get current fan speed on second attempt: %w", err)
		}
	}

	lastFanSpeed = currentFanSpeed

	debugLog("Initial fan speed: %d%%", lastFanSpeed)
	return nil
}

func initializePowerLimits() error {
	if err := getMinMaxPowerLimits(); err != nil {
		return fmt.Errorf("failed to get power limits: %w", err)
	}

	var err error
	currentPowerLimit, err = getCurrentPowerLimit()
	if err != nil {
		return fmt.Errorf("failed to get current power limit: %w", err)
	}
	cachePowerLimits = true

	debugLog("Initial power limit: %dW", currentPowerLimit)
	return nil
}

func main() {
	if err := initNvml(); err != nil {
		log.Fatalf("Initialization failed: %v", err)
	}
	// Ensure we shutdown even if there's a panic
	if nvmlInitialized {
		defer nvml.Shutdown()
	}

	// Set up a channel to receive OS signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Use a WaitGroup to ensure all goroutines finish before exiting
	var wg sync.WaitGroup

	// Start the main loop in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop(ctx)
	}()

	// Wait for termination signal
	<-sigs
	log.Println("Received termination signal.")

	// Cancel the context and wait for the main loop to finish
	cancel()
	wg.Wait()

	// Re-enable auto fan control
	enableAutoFanControl()
	if config.Debug {
		log.Println("Auto fan control re-enabled.")
	}

	// Reset power limit to default
	setPowerLimit(defaultPowerLimit)
	debugLog("Power limit reset to default.")

	log.Println("Exiting...")
}

func loop(ctx context.Context) {
	ticker := time.NewTicker(time.Second * time.Duration(config.Interval))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			setLastFanSpeed()

			currentTemp := getCurrentTemperature()
			targetFanSpeed := calculateFanSpeed(currentTemp, config.Temperature, config.FanSpeed)

			if !config.Monitor {
				setFanSpeed(targetFanSpeed)
			}

			if !config.PerformanceMode {
				powerAdjustment := calculatePowerLimit(currentTemp, config.Temperature)
				newPowerLimit := currentPowerLimit - powerAdjustment

				// Ensure the new power limit is within the allowed range
				newPowerLimit = clamp(newPowerLimit, minPowerLimit, maxPowerLimit)

				if newPowerLimit != currentPowerLimit {
					currentPowerLimit = newPowerLimit
					setPowerLimit(currentPowerLimit)
				}
			} else if currentPowerLimit < maxPowerLimit {
				// In performance mode, set power limit to maximum
				setPowerLimit(maxPowerLimit)
			}

			if config.Debug {
				actualFanSpeed, _ := getCurrentFanSpeed()
				logger.Printf("  Temperature: current=%d°C, max=%d°C", currentTemp, config.Temperature)
				logger.Printf("  Fan Speed: current=%d%%, target=%d%%, last=%d%%, max=%d%%",
					actualFanSpeed, targetFanSpeed, lastFanSpeed, config.FanSpeed)
				logger.Printf("  Power Limit: current=%dW, min=%dW, max=%dW",
					currentPowerLimit, minPowerLimit, maxPowerLimit)
				logger.Printf("  Fan Limits: min=%d%%, max=%d%%", minFanSpeedLimit, maxFanSpeedLimit)
				logger.Printf("  Settings: hysteresis=%d, monitor=%v, performance=%v",
					config.Hysteresis, config.Monitor, config.PerformanceMode)
			} else {
				logger.Printf("Temperature: %d°C, Fan Speed: %d%%, Power Limit: %dW",
					currentTemp, targetFanSpeed, currentPowerLimit)
			}
		}
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

func isRunningUnderSystemd() bool {
	return os.Getenv("JOURNAL_STREAM") != ""
}

func debugLog(format string, v ...interface{}) {
	if config.Debug {
		logger.Printf("Debug: "+format, v...)
	}
}

// Main functions
func getDeviceHandle() (nvml.Device, error) {
	var initErr error
	deviceSync.Do(func() {
		var ret nvml.Return
		cacheDevice, ret = nvml.DeviceGetHandleByUUID(gpuUUID)
		if ret != nvml.SUCCESS {
			initErr = fmt.Errorf("failed to get device handle: %v", nvml.ErrorString(ret))
		}
	})
	return cacheDevice, initErr
}

func getCurrentTemperature() int {
	device, _ := getDeviceHandle()
	temp, _ := device.GetTemperature(nvml.TEMPERATURE_GPU)
	return int(temp)
}

func getCurrentFanSpeed() (int, error) {
	device, err := getDeviceHandle()
	if err != nil {
		return 0, fmt.Errorf("failed to get device handle: %w", err)
	}

	fanSpeed, ret := device.GetFanSpeed()
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("failed to get fan speed: %v", nvml.ErrorString(ret))
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
		return fmt.Errorf("failed to get min/max fan speed: %v", nvml.ErrorString(ret))
	}

	minFanSpeedLimit = int(minSpeed)
	maxFanSpeedLimit = int(maxSpeed)

	// Cache the results
	cacheFanSpeeds = true

	debugLog("Fan speed limits detected. Min: %d%%, Max: %d%%", minFanSpeedLimit, maxFanSpeedLimit)

	return nil
}

func calculateFanSpeed(currentTemp, maxTemp, maxFanSpeed int) int {
	// Increase the fans to max as a precaution, in case temperature rises above maxTemp
	if currentTemp > maxTemp {
		debugLog("Temperature (%d°C) at or above max (%d°C). Setting fan speed to max (%d%%).", currentTemp, maxTemp, maxFanSpeed)
		return maxFanSpeed
	}

	tempRange := float64(maxTemp - minTemp)
	tempPercentage := float64(currentTemp-minTemp) / tempRange

	var fanSpeedPercentage float64
	if config.PerformanceMode {
		// More aggressive curve for performance mode
		fanSpeedPercentage = math.Pow(tempPercentage, 1.5)
	} else {
		// Original curve for normal mode
		fanSpeedPercentage = math.Pow(tempPercentage, 2)
	}

	var targetFanSpeed int
	targetFanSpeed = int(float64(maxFanSpeed-minFanSpeedLimit)*fanSpeedPercentage) + minFanSpeedLimit
	targetFanSpeed = clamp(targetFanSpeed, minFanSpeedLimit, maxFanSpeed)

	if applyHysteresis(targetFanSpeed, lastFanSpeed, config.Hysteresis, config.PerformanceMode) {
		debugLog("Hysteresis applied. Keeping lastFanSpeed: %d%%", lastFanSpeed)
		return lastFanSpeed
	}

	debugLog("New target fan speed: %d%%", targetFanSpeed)
	return targetFanSpeed
}

func setFanSpeed(fanSpeed int) {
	device, _ := getDeviceHandle()

	debugLog("Attempting to set fan speed to %d%%", fanSpeed)

	currentTemp := getCurrentTemperature()
	if currentTemp <= minTemp {
		debugLog("Temperature (%d°C) below fan-off threshold (%d°C). Enabling auto fan control.", currentTemp, minTemp)
		enableAutoFanControl()
	} else {
		ret := nvml.DeviceSetFanSpeed_v2(device, 0, fanSpeed) // Assuming 0 is the first fan
		if ret != nvml.SUCCESS {
			logger.Printf("Error setting fan speed: %v", nvml.ErrorString(ret))
		}
	}

	currentFanSpeed, _ := getCurrentFanSpeed()
	debugLog("Current fan speed after setting: %d%%", currentFanSpeed)
}

func applyHysteresis(newSpeed, lastSpeed, hysteresis int, performanceMode bool) bool {
	if performanceMode {
		// In performance mode, prioritize GPU performance over noise reduction
		if newSpeed > lastSpeed {
			return newSpeed <= lastSpeed+hysteresis
		}
		return newSpeed >= lastSpeed-hysteresis*2
	}
	// Original hysteresis logic for normal mode
	return (newSpeed >= lastSpeed-hysteresis) && (newSpeed <= lastSpeed+hysteresis)
}

func setLastFanSpeed() {
	currentSpeed, _ := getCurrentFanSpeed()
	lastFanSpeed = currentSpeed
	debugLog("Updated lastFanSpeed: %d%%", lastFanSpeed)
}

func enableAutoFanControl() {
	device, _ := getDeviceHandle()

	ret := nvml.DeviceSetDefaultFanSpeed_v2(device, 0)
	if ret != nvml.SUCCESS {
		logger.Printf("Error setting default fan speed: %v", nvml.ErrorString(ret))
	}
}

func getCurrentPowerLimit() (int, error) {
	device, err := getDeviceHandle()
	if err != nil {
		return 0, fmt.Errorf("failed to get device handle: %w", err)
	}

	powerLimit, ret := device.GetPowerManagementLimit()
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("failed to get current power limit: %v", nvml.ErrorString(ret))
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
		return fmt.Errorf("failed to get power management limit constraints: %v", nvml.ErrorString(ret))
	}

	defaultLimit, ret := device.GetPowerManagementDefaultLimit()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to get default power management limit: %v", nvml.ErrorString(ret))
	}

	minPowerLimit = int(minLimit / 1000) // Convert from milliwatts to watts
	maxPowerLimit = int(maxLimit / 1000)
	defaultPowerLimit = int(defaultLimit / 1000)

	debugLog("Power limits detected. Min: %dW, Max: %dW, Default: %dW", minPowerLimit, maxPowerLimit, defaultPowerLimit)

	// Cache the results
	cachePowerLimits = true

	return nil
}

func calculatePowerLimit(currentTemp, targetTemp int) int {
	tempDiff := currentTemp - targetTemp
	adjustment := tempDiff * wattPerDegree
	return clamp(adjustment, -maxPowerLimitAdjustment, maxPowerLimitAdjustment)
}

func setPowerLimit(powerLimit int) error {
	device, _ := getDeviceHandle()

	ret := device.SetPowerManagementLimit(uint32(powerLimit * 1000)) // Convert watts to milliwatts
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to set power limit: %v", nvml.ErrorString(ret))
	}

	debugLog("Power limit set successfully to %dW", powerLimit)
	return nil
}
