package metrics

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"time"

	"codeberg.org/mutker/nvidiactl/internal/errors"
	"codeberg.org/mutker/nvidiactl/internal/logger"
	_ "github.com/mattn/go-sqlite3"
)

type repository struct {
	db            *sql.DB
	logger        logger.Logger
	cfg           Config
	mu            sync.Mutex
	buffer        []*MetricsSnapshot
	flushTicker   *time.Ticker
	shutdownChan  chan struct{}
	flushDoneChan chan struct{}
}

func NewRepository(cfg Config, log logger.Logger) (MetricsRepository, error) {
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
	dsn := cfg.DBPath + "?_journal=WAL&_auto_vacuum=2"
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
	if err := ValidateAndUpdateSchema(db, log); err != nil {
		db.Close()
		return nil, errFactory.WithData(ErrStorageInit, struct {
			Phase string
			Error string
		}{
			Phase: "schema_version",
			Error: err.Error(),
		})
	}

	log.Info().
		Str("path", cfg.DBPath).
		Int("schema_version", SchemaVersion).
		Int("batch_size", cfg.BatchSize).
		Int("batch_timeout", cfg.BatchTimeout).
		Msg("Metrics repository initialized")

	repo := &repository{
		db:            db,
		logger:        log,
		cfg:           cfg,
		buffer:        make([]*MetricsSnapshot, 0, cfg.BatchSize),
		shutdownChan:  make(chan struct{}),
		flushDoneChan: make(chan struct{}),
	}

	// Start background goroutine for periodic flushing if batching is enabled
	if cfg.BatchSize > 0 && cfg.BatchTimeout > 0 {
		repo.flushTicker = time.NewTicker(time.Duration(cfg.BatchTimeout) * time.Second)
		go repo.flusher()
	}

	return repo, nil
}

func (r *repository) Record(snapshot *MetricsSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.buffer = append(r.buffer, snapshot)

	if len(r.buffer) >= r.cfg.BatchSize {
		return r.flush()
	}

	return nil
}

func (r *repository) Close() error {
	// Signal the flusher goroutine to stop
	close(r.shutdownChan)

	// Stop the ticker
	r.flushTicker.Stop()

	// Wait for the flusher to finish its final flush
	<-r.flushDoneChan

	// Checkpoint WAL and cleanup on close
	if _, err := r.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return errors.New().WithData(ErrStorageClose, struct {
			Phase string
			Error string
		}{
			Phase: "checkpoint_wal",
			Error: err.Error(),
		})
	}

	if err := r.db.Close(); err != nil {
		return errors.New().WithData(ErrStorageClose, struct {
			Phase string
			Error string
		}{
			Phase: "close_database",
			Error: err.Error(),
		})
	}

	r.logger.Info().Msg("Metrics repository closed gracefully")

	return nil
}

func (r *repository) flusher() {
	defer close(r.flushDoneChan)

	for {
		select {
		case <-r.flushTicker.C:
			r.mu.Lock()
			r.flush()
			r.mu.Unlock()
		case <-r.shutdownChan:
			r.mu.Lock()
			r.flush()
			r.mu.Unlock()
			return
		}
	}
}

func (r *repository) flush() error {
	if len(r.buffer) == 0 {
		return nil
	}

	errFactory := errors.New()

	tx, err := r.db.Begin()
	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to begin transaction")
		return errFactory.Wrap(ErrTransactionFailed, err)
	}

	stmt, err := tx.Prepare(GetInsertMetricSQL())
	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to prepare statement")
		if err := tx.Rollback(); err != nil {
			r.logger.Error().Err(err).Msg("Failed to roll back transaction")
		}
		return errFactory.Wrap(ErrTransactionFailed, err)
	}
	defer stmt.Close()

	for _, snapshot := range r.buffer {
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

		if _, err := stmt.Exec(values...); err != nil {
			r.logger.Error().Err(err).Msg("Failed to execute insert")
			if err := tx.Rollback(); err != nil {
				r.logger.Error().Err(err).Msg("Failed to roll back transaction")
			}
			return errFactory.Wrap(ErrTransactionFailed, err)
		}
	}

	if err := tx.Commit(); err != nil {
		r.logger.Error().Err(err).Msg("Failed to commit transaction")
		return errFactory.Wrap(ErrTransactionFailed, err)
	}

	r.logger.Debug().Int("records", len(r.buffer)).Msg("Flushed metrics to database")
	r.buffer = r.buffer[:0]

	return nil
}
