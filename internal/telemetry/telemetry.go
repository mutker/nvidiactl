package telemetry

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"time"

	"codeberg.org/mutker/nvidiactl/internal/errors"
	"codeberg.org/mutker/nvidiactl/internal/logger"

	// sqlite3 driver is imported for its side effects
	_ "github.com/mattn/go-sqlite3"
)

const (
	defaultDirPerm = 0o755
	defaultDBPath  = "/var/lib/nvidiactl/telemetry.db"
)

type Database struct {
	db   *sql.DB
	path string
	mu   sync.Mutex
}

type Metrics struct {
	Timestamp          time.Time
	FanSpeed           int
	TargetFanSpeed     int
	Temperature        int
	AverageTemperature int
	PowerLimit         int
	TargetPowerLimit   int
	AveragePowerLimit  int
	AutoFanControl     bool
	PerformanceMode    bool
}

func NewTelemetryDB(dbPath string) (*Database, error) {
	if dbPath == "" {
		dbPath = defaultDBPath
	}

	logger.Debug().Msgf("Telemetry DB path: %s", dbPath)

	// Ensure the directory exists
	err := os.MkdirAll(filepath.Dir(dbPath), defaultDirPerm)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create directory for telemetry DB")
		return nil, errors.Wrap(errors.ErrInitTelemetry, err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to open telemetry DB")
		return nil, errors.Wrap(errors.ErrInitTelemetry, err)
	}

	telemetry := &Database{
		db:   db,
		path: dbPath,
	}

	if err := InitSchema(db); err != nil {
		logger.Error().Err(err).Msg("Failed to initialize telemetry DB schema")
		return nil, err
	}

	logger.Debug().Msg("Telemetry DB initialized successfully")

	return telemetry, nil
}

func (t *Database) CollectMetrics(metrics *Metrics) {
	t.mu.Lock()
	defer t.mu.Unlock()

	_, err := t.db.Exec(`
        INSERT INTO telemetry (
            timestamp, fan_speed, target_fan_speed,
            temperature, average_temperature,
            power_limit, target_power_limit, average_power_limit,
            auto_fan_control, performance_mode
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(timestamp) DO UPDATE SET
            fan_speed = excluded.fan_speed,
            target_fan_speed = excluded.target_fan_speed,
            temperature = excluded.temperature,
            average_temperature = excluded.average_temperature,
            power_limit = excluded.power_limit,
            target_power_limit = excluded.target_power_limit,
            average_power_limit = excluded.average_power_limit,
            auto_fan_control = excluded.auto_fan_control,
            performance_mode = excluded.performance_mode
    `,
		metrics.Timestamp.Unix(), metrics.FanSpeed, metrics.TargetFanSpeed,
		metrics.Temperature, metrics.AverageTemperature,
		metrics.PowerLimit, metrics.TargetPowerLimit, metrics.AveragePowerLimit,
		boolToInt(metrics.AutoFanControl), boolToInt(metrics.PerformanceMode),
	)
	if err != nil {
		logger.Error().Err(err).Msg(errors.ErrorMessage(errors.ErrCollectTelemetry))
	} else {
		logger.Debug().Msg("Telemetry metrics collected successfully")
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}

	return 0
}

func (t *Database) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	err := t.db.Close()
	if err != nil {
		return errors.Wrap(errors.ErrCloseTelemetry, err)
	}

	return nil
}
