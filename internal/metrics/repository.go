package metrics

import (
	"database/sql"
	"os"
	"path/filepath"

	"codeberg.org/mutker/nvidiactl/internal/errors"
	"codeberg.org/mutker/nvidiactl/internal/logger"
	_ "github.com/mattn/go-sqlite3"
)

type repository struct {
	db         *sql.DB
	insertStmt *sql.Stmt
}

func NewRepository(cfg Config) (MetricsRepository, error) {
	errFactory := errors.New()

	if cfg.DBPath == "" {
		return nil, errFactory.New(ErrInvalidDBPath)
	}

	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), defaultDirPerm); err != nil {
		return nil, errFactory.WithData(ErrStorageInit, struct {
			Phase string
			Path  string
			Error string
		}{
			Phase: "create_directory",
			Path:  cfg.DBPath,
			Error: err.Error(),
		})
	}

	// Open database with specific pragmas for better performance and safety
	dsn := cfg.DBPath + "?_journal=WAL&_timeout=5000&_sync=NORMAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, errFactory.WithData(ErrStorageInit, struct {
			Phase string
			Error string
		}{
			Phase: "open_database",
			Error: err.Error(),
		})
	}

	// Validate if schema is current, with backup if needed
	if err := ValidateAndUpdateSchema(db); err != nil {
		db.Close()
		return nil, errFactory.WithData(ErrStorageInit, struct {
			Phase string
			Error string
		}{
			Phase: "schema_version",
			Error: err.Error(),
		})
	}

	// Prepare insert statement
	stmt, err := db.Prepare(GetInsertMetricSQL())
	if err != nil {
		db.Close()
		return nil, errFactory.WithData(ErrStorageInit, struct {
			Phase string
			Error string
		}{
			Phase: "prepare_statement",
			Error: err.Error(),
		})
	}

	logger.Info().
		Str("path", cfg.DBPath).
		Int("schema_version", SchemaVersion).
		Msg("Metrics repository initialized")

	return &repository{
		db:         db,
		insertStmt: stmt,
	}, nil
}

func (r *repository) Record(snapshot *MetricsSnapshot) error {
	errFactory := errors.New()

	values := []interface{}{
		snapshot.Timestamp.Unix(),
		int64(snapshot.FanSpeed.Current),
		int64(snapshot.FanSpeed.Target),
		int64(snapshot.Temperature.Current),
		int64(snapshot.Temperature.Average),
		int64(snapshot.PowerLimit.Current),
		int64(snapshot.PowerLimit.Target),
		int64(snapshot.PowerLimit.Average),
		int64(boolToInt(snapshot.SystemState.AutoFanControl)),
		int64(boolToInt(snapshot.SystemState.PerformanceMode)),
	}

	if _, err := r.insertStmt.Exec(values...); err != nil {
		return errFactory.WithData(ErrStorageAccess, struct {
			Phase  string
			Error  string
			Values interface{}
		}{
			Phase:  "execute_insert",
			Error:  err.Error(),
			Values: values,
		})
	}

	return nil
}

func (r *repository) Close() error {
	errFactory := errors.New()

	// Close prepared statement
	if err := r.insertStmt.Close(); err != nil {
		logger.Debug().Err(err).Msg("Failed to close prepared statement")
	}

	// Checkpoint WAL and cleanup on close
	if _, err := r.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return errFactory.WithData(ErrStorageClose, struct {
			Phase string
			Error string
		}{
			Phase: "checkpoint_wal",
			Error: err.Error(),
		})
	}

	if err := r.db.Close(); err != nil {
		return errFactory.WithData(ErrStorageClose, struct {
			Phase string
			Error string
		}{
			Phase: "close_database",
			Error: err.Error(),
		})
	}

	return nil
}
