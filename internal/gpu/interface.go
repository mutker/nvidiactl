package gpu

// Controller manages GPU operations and state
type Controller interface {
	// Core operations
	Initialize() error
	Shutdown() error

	// Temperature management
	GetTemperature() (Temperature, error)
	GetAverageTemperature() Temperature
	UpdateTemperatureHistory(Temperature) Temperature

	// Fan control
	GetFanControl() FanController
	EnableAutoFanControl() error
	DisableAutoFanControl() error
	GetCurrentFanSpeeds() []FanSpeed
	SetFanSpeed(speed FanSpeed) error
	GetLastFanSpeeds() []FanSpeed
	GetFanSpeedLimits() FanSpeedLimits

	// Power management
	GetPowerControl() PowerController
	GetCurrentPowerLimit() PowerLimit
	SetPowerLimit(PowerLimit) error
	GetPowerLimits() PowerLimits
	UpdatePowerLimitHistory(PowerLimit) PowerLimit
}

// FanController manages fan operations
type FanController interface {
	GetSpeed(fanIndex int) (FanSpeed, error)
	GetCurrentSpeeds() []FanSpeed
	GetSpeedLimits() FanSpeedLimits
	EnableAuto() error
	DisableAuto() error
	SetSpeed(speed FanSpeed) error
	IsAutoMode() bool
	GetLastSpeeds() []FanSpeed
}

// PowerController manages power operations
type PowerController interface {
	GetLimit() (PowerLimit, error)
	SetLimit(limit PowerLimit) error
	GetLimits() PowerLimits
	GetLastLimit() PowerLimit
	GetCurrentLimit() PowerLimit
	ResetToDefault() error
	UpdateHistory(limit PowerLimit) PowerLimit
}

// Domain types for type safety and validation
type (
	Temperature int
	FanSpeed    int
	PowerLimit  int

	FanSpeedLimits struct {
		Min, Max, Default FanSpeed
	}

	PowerLimits struct {
		Min, Max, Default PowerLimit
	}
)
