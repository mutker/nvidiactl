package telemetry

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	"codeberg.org/mutker/nvidiactl/internal/errors"
	_ "github.com/mattn/go-sqlite3"
)

type repository struct {
	db *sql.DB
}

func NewRepository(cfg Config) (Repository, error) {
	errFactory := errors.New()

	if cfg.DBPath == "" {
		return nil, errFactory.New(ErrInvalidDBPath)
	}

	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), defaultDirPerm); err != nil {
		return nil, errFactory.Wrap(ErrStorageInit, err)
	}

	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		return nil, errFactory.Wrap(ErrStorageInit, err)
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, errFactory.Wrap(ErrStorageInit, err)
	}

	return &repository{db: db}, nil
}

func (r *repository) Record(snapshot *MetricsSnapshot) error {
	errFactory := errors.New()
	stmt, err := r.db.Prepare(`
        INSERT INTO metrics (
            timestamp,
            fan_speed_current, fan_speed_target,
            temp_current, temp_average,
            power_current, power_target, power_average,
            auto_fan_control, performance_mode
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `)
	if err != nil {
		return errFactory.Wrap(ErrStorageAccess, err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		snapshot.Timestamp.Format(time.RFC3339),
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
		return errFactory.Wrap(ErrStorageAccess, err)
	}

	return nil
}

func (r *repository) Close() error {
	errFactory := errors.New()

	if err := r.db.Close(); err != nil {
		return errFactory.Wrap(ErrStorageClose, err)
	}
	return nil
}
