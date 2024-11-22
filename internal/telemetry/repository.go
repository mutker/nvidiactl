package telemetry

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"

	"codeberg.org/mutker/nvidiactl/internal/errors"
	"codeberg.org/mutker/nvidiactl/internal/logger"

	_ "github.com/mattn/go-sqlite3"
)

type Repository interface {
	Store(ctx context.Context, snapshot *MetricsSnapshot) error
	Close() error
}

type sqliteRepository struct {
	db *sql.DB
	mu sync.Mutex
}

func NewRepository(cfg Config) (Repository, error) {
	if cfg.DBPath == "" {
		return nil, errors.New(ErrInvalidDBPath)
	}

	logger.Debug().Msgf("Initializing telemetry repository at: %s", cfg.DBPath)

	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), defaultDirPerm); err != nil {
		return nil, errors.Wrap(ErrStorageInit, err)
	}

	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		return nil, errors.Wrap(ErrStorageInit, err)
	}

	// Initialize schema
	if err := initSchema(db); err != nil { // lowercase initSchema
		db.Close()
		return nil, errors.Wrap(ErrStorageInit, err)
	}

	return &sqliteRepository{
		db: db,
	}, nil
}

func (r *sqliteRepository) Store(ctx context.Context, snapshot *MetricsSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.ExecContext(ctx, `
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
		snapshot.Timestamp.Unix(),
		snapshot.FanSpeed.Current,
		snapshot.FanSpeed.Target,
		snapshot.Temperature.Current,
		snapshot.Temperature.Average,
		snapshot.PowerLimit.Current,
		snapshot.PowerLimit.Target,
		snapshot.PowerLimit.Average,
		boolToInt(snapshot.SystemState.AutoFanControl),
		boolToInt(snapshot.SystemState.PerformanceMode),
	)
	if err != nil {
		return errors.Wrap(ErrStorageAccess, err)
	}

	return nil
}

func (r *sqliteRepository) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.db.Close(); err != nil {
		return errors.Wrap(ErrStorageClose, err)
	}
	return nil
}
