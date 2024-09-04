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
	// LogFile     string
	Interval        int
	Temp            int
	FanSpeed        int
	Hysteresis      int
	PerformanceMode bool
	Monitor         bool
	Debug           bool
}

const (
	minTemp = 50
)

var (
	nvmlInitialized   bool
	cachedDevice      nvml.Device
	deviceSync        sync.Once
	gpuUUID           string
	config            Config
	interval          int
	minFanSpeedLimit  int
	maxFanSpeedLimit  int
	lastFanSpeed      int
	fanSpeedsCached   bool
	defaultPowerLimit int
	minPowerLimit     int
	maxPowerLimit     int
	currentPowerLimit int
	powerLimitsCached bool
)

func init() {
	log.Println("Initializing nvidiactl...")

	// flag.StringVar(&config.LogFile, "log", "nvidiactl.log", "Path to log file")
	flag.IntVar(&config.Temp, "temperature", 80, "Maximum allowed temperature")
	flag.IntVar(&config.Interval, "interval", 2, "Interval between updates")
	flag.IntVar(&config.FanSpeed, "fanspeed", 100, "Maximum allowed fan speed")
	flag.IntVar(&config.Hysteresis, "hysteresis", 2, "Temperature hysteresis")
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
		log.Printf("Using config file: %s", viper.ConfigFileUsed())
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
					log.Printf("	%s: %v", field.Name, value)
				}
			} else if field.Name == "PerformanceMode" {
				log.Printf("	%s: %v", "Performance mode", value)
			} else if field.Name == "FanSpeed" {
				log.Printf("	%s: %v", "Fan speed", value)
			} else if field.Name != "LogFile" {
				log.Printf("	%s: %v", field.Name, value)
			}
		}
	}
}

func initNvml() error {
	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("unable to initialize NVML: %v", nvml.ErrorString(ret))
	}
	nvmlInitialized = true
	defer func() {
		if !nvmlInitialized {
			nvml.Shutdown()
		}
	}()

	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("unable to get device count: %v", nvml.ErrorString(ret))
	}

	if count == 0 {
		return fmt.Errorf("no NVIDIA GPUs found")
	}

	// We'll use the first GPU (index 0)
	device, ret := nvml.DeviceGetHandleByIndex(0)
	if ret != nvml.SUCCESS {
		return fmt.Errorf("unable to get device at index 0: %v", nvml.ErrorString(ret))
	}

	uuid, ret := device.GetUUID()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("unable to get UUID of device: %v", nvml.ErrorString(ret))
	}

	name, ret := device.GetName()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("unable to get name of device: %v", nvml.ErrorString(ret))
	}

	gpuUUID = uuid
	log.Printf("Detected GPU: %s\n", name)

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

	currentFanSpeed, err := getCurrentFanSpeed()
	if err != nil {
		return fmt.Errorf("failed to get current fan speed: %w", err)
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
	powerLimitsCached = true

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

			currentTemp := getTemperature()
			targetFanSpeed := calculateFanSpeed(currentTemp, config.Temp, config.FanSpeed)

			if !config.Monitor {
				setFanSpeed(targetFanSpeed)
			}

			if !config.PerformanceMode {
				powerAdjustment := calculatePowerLimit(currentTemp, config.Temp)
				newPowerLimit := currentPowerLimit - powerAdjustment

				// Ensure the new power limit is within the allowed range
				newPowerLimit = max(min(newPowerLimit, maxPowerLimit), minPowerLimit)

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
				log.Printf("  Temperature: current=%d°C, max=%d°C", currentTemp, config.Temp)
				log.Printf("  Fan Speed: current=%d%%, target=%d%%, last=%d%%, max=%d%%",
					actualFanSpeed, targetFanSpeed, lastFanSpeed, config.FanSpeed)
				log.Printf("  Power Limit: current=%dW, min=%dW, max=%dW",
					currentPowerLimit, minPowerLimit, maxPowerLimit)
				log.Printf("  Fan Limits: min=%d%%, max=%d%%", minFanSpeedLimit, maxFanSpeedLimit)
				log.Printf("  Settings: hysteresis=%d, monitor=%v, performance=%v",
					config.Hysteresis, config.Monitor, config.PerformanceMode)
			} else {
				log.Printf("Temperature: %d°C, Fan Speed: %d%%, Power Limit: %dW",
					currentTemp, targetFanSpeed, currentPowerLimit)
			}
		}
	}
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func debugLog(format string, v ...interface{}) {
	if config.Debug {
		log.Printf("Debug: "+format, v...)
	}
}

// Main functions
func getDeviceHandle() (nvml.Device, error) {
	var initErr error
	deviceSync.Do(func() {
		var ret nvml.Return
		cachedDevice, ret = nvml.DeviceGetHandleByUUID(gpuUUID)
		if ret != nvml.SUCCESS {
			initErr = fmt.Errorf("Failed to get device handle: %v", nvml.ErrorString(ret))
		}
	})
	return cachedDevice, initErr
}

func getTemperature() int {
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
	if fanSpeedsCached {
		return nil
	}

	device, _ := getDeviceHandle()
	minSpeed, maxSpeed, ret := device.GetMinMaxFanSpeed()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("Failed to get min/max fan speed: %v", nvml.ErrorString(ret))
	}

	minFanSpeedLimit = int(minSpeed)
	maxFanSpeedLimit = int(maxSpeed)

	// Cache the results
	fanSpeedsCached = true

	debugLog("Fan speed limits detected. Min: %d%%, Max: %d%%", minFanSpeedLimit, maxFanSpeedLimit)

	return nil
}

func calculateFanSpeed(currentTemp, maxTemp, maxFanSpeed int) int {

	if currentTemp <= minTemp {
		debugLog("Temperature (%d°C) below fan-off threshold (%d°C). Enabling auto fan control.", currentTemp, minTemp)
		enableAutoFanControl()
	}

	if currentTemp >= maxTemp {
		debugLog("Temperature (%d°C) at or above max (%d°C). Setting fan speed to max (%d%%).", currentTemp, maxTemp, maxFanSpeed)
		return maxFanSpeed
	}

	// Calculate the temperature range, starting from the fan-off temperature
	tempRange := maxTemp - minTemp

	// Calculate the percentage of where the current temperature is in the range
	tempPercentage := float64(currentTemp-minTemp) / float64(tempRange)

	// Use a curve to calculate fan speed
	// This will make the fan speed increase faster as it gets closer to the max temp
	fanSpeedPercentage := math.Pow(tempPercentage, 2)

	// Calculate the target fan speed
	var targetFanSpeed int
	if currentTemp < minTemp {
		// Linear interpolation between 0 and minFanSpeedLimit
		targetFanSpeed = int(float64(minFanSpeedLimit) * (float64(currentTemp-minTemp) / float64(minTemp-minTemp)))
	} else {
		// Use the curve calculation
		targetFanSpeed = int(float64(maxFanSpeed-minFanSpeedLimit)*fanSpeedPercentage) + minFanSpeedLimit
	}

	// Ensure the fan speed is within the valid range
	targetFanSpeed = max(min(targetFanSpeed, maxFanSpeed), 0)

	// Apply hysteresis
	if applyHysteresis(targetFanSpeed, lastFanSpeed, config.Hysteresis) {
		debugLog("Hysteresis applied. Keeping lastFanSpeed: %d%%", lastFanSpeed)
		return lastFanSpeed
	}

	debugLog("New target fan speed: %d%%", targetFanSpeed)
	lastFanSpeed = targetFanSpeed
	return targetFanSpeed
}

func setFanSpeed(fanSpeed int) {
	device, _ := getDeviceHandle()

	debugLog("Attempting to set fan speed to %d%%", fanSpeed)

	if fanSpeed == 0 {
		enableAutoFanControl()
	} else {
		ret := nvml.DeviceSetFanSpeed_v2(device, 0, fanSpeed) // Assuming 0 is the first fan
		if ret != nvml.SUCCESS {
			log.Printf("Error setting fan speed: %v", nvml.ErrorString(ret))
		}
	}
	currentFanSpeed, _ := getCurrentFanSpeed()
	debugLog("Current fan speed after setting: %d%%", currentFanSpeed)
}

func applyHysteresis(newSpeed, lastSpeed, hysteresis int) bool {
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
		log.Printf("Error setting default fan speed: %v", nvml.ErrorString(ret))
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
	if powerLimitsCached {
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
	powerLimitsCached = true

	return nil
}

func calculatePowerLimit(currentTemp, targetTemp int) int {
	tempDiff := currentTemp - targetTemp

	// Define the maximum adjustment per cycle (e.g., 20W)
	maxAdjustment := 20

	// Calculate the adjustment proportionally
	adjustment := tempDiff * 2 // 2W per degree of difference

	// Clamp the adjustment to the maximum value
	if adjustment > maxAdjustment {
		adjustment = maxAdjustment
	} else if adjustment < -maxAdjustment {
		adjustment = -maxAdjustment
	}

	return adjustment
}

func setPowerLimit(powerLimit int) error {
	device, _ := getDeviceHandle()

	ret := device.SetPowerManagementLimit(uint32(powerLimit * 1000)) // Convert watts to milliwatts
	if ret != nvml.SUCCESS {
		return fmt.Errorf("Failed to set power limit: %v", nvml.ErrorString(ret))
	}

	debugLog("Power limit set successfully to %dW", powerLimit)
	return nil
}
