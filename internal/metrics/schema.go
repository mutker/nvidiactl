package metrics

import (
	"database/sql"

	"codeberg.org/mutker/nvidiactl/internal/errors"
	"codeberg.org/mutker/nvidiactl/internal/logger"
)

const (
	SchemaVersion = 2 // Increment version for breaking change

	// SQL statements derived from schema
	createTablesSQL = `
	   CREATE TABLE IF NOT EXISTS schema_versions (
	       version     INTEGER PRIMARY KEY,
	       applied_at  TEXT NOT NULL
	   );
	   CREATE TABLE IF NOT EXISTS metrics (
	       timestamp        INTEGER PRIMARY KEY,
	       fan_speed_current INTEGER NOT NULL CHECK (typeof(fan_speed_current) = 'integer'),
	       fan_speed_target  INTEGER NOT NULL CHECK (typeof(fan_speed_target) = 'integer'),
	       temp_current     INTEGER NOT NULL CHECK (typeof(temp_current) = 'integer'),
	       temp_average     INTEGER NOT NULL CHECK (typeof(temp_average) = 'integer'),
	       power_current    INTEGER NOT NULL CHECK (typeof(power_current) = 'integer'),
	       power_target     INTEGER NOT NULL CHECK (typeof(power_target) = 'integer'),
	       power_average    INTEGER NOT NULL CHECK (typeof(power_average) = 'integer'),
	       auto_fan_control INTEGER NOT NULL CHECK (auto_fan_control IN (0, 1)),
	       performance_mode INTEGER NOT NULL CHECK (performance_mode IN (0, 1))
	   );`

	insertMetricsSQL = `
    INSERT INTO metrics (
        timestamp,
        fan_speed_current, fan_speed_target,
        temp_current, temp_average,
        power_current, power_target, power_average,
        auto_fan_control, performance_mode
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
)

// InitSchema creates a new database schema with the current version
func InitSchema(db *sql.DB, log logger.Logger) error {
	errFactory := errors.New()

	log.Debug().Msg("Creating database...")

	tx, err := db.Begin()
	if err != nil {
		return errFactory.Wrap(ErrSchemaInitFailed, err)
	}

	// Track transaction state
	committed := false
	defer func() {
		if !committed {
			if err := tx.Rollback(); err != nil {
				// Only log if it's not the "already committed" error
				if !errors.Is(err, sql.ErrTxDone) {
					log.Debug().Err(err).Msg("Failed to rollback transaction")
				}
			}
		}
	}()

	// Execute schema creation
	log.Debug().Str("sql", createTablesSQL).Msg("Executing SQL statement")
	if _, err := tx.Exec(createTablesSQL); err != nil {
		return errFactory.WithData(ErrSchemaInitFailed, struct {
			Error string
			SQL   string
		}{
			Error: err.Error(),
			SQL:   createTablesSQL,
		})
	}

	log.Debug().Msg("Recording schema version...")
	// Record schema version
	if _, err := tx.Exec(`
        INSERT INTO schema_versions (version, applied_at)
        VALUES (?, datetime('now'))
    `, SchemaVersion); err != nil {
		return errFactory.WithData(ErrSchemaInitFailed, struct {
			Error string
			Phase string
		}{
			Error: err.Error(),
			Phase: "record_version",
		})
	}

	log.Debug().Msg("Committing transaction...")
	if err := tx.Commit(); err != nil {
		return errFactory.Wrap(ErrSchemaInitFailed, err)
	}
	committed = true

	log.Info().
		Int("version", SchemaVersion).
		Msg("Schema initialized successfully")

	return nil
}

// GetSchemaVersion returns the current schema version
func GetSchemaVersion(db *sql.DB) (int, error) {
	errFactory := errors.New()

	exists, err := TableExists(db, "schema_versions")
	if err != nil {
		return 0, errFactory.Wrap(ErrSchemaValidationFailed, err)
	}
	if !exists {
		return 0, nil
	}

	var version int
	err = db.QueryRow(`
        SELECT version
        FROM schema_versions
        ORDER BY version DESC
        LIMIT 1
    `).Scan(&version)

	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, errFactory.WithData(ErrSchemaValidationFailed, struct {
			Phase string
			Error string
		}{
			Phase: "get_version",
			Error: err.Error(),
		})
	}

	return version, nil
}

// TableExists checks if a table exists
func TableExists(db *sql.DB, tableName string) (bool, error) {
	errFactory := errors.New()
	var exists bool
	err := db.QueryRow(`
        SELECT EXISTS (
            SELECT 1 FROM sqlite_master
            WHERE type='table' AND name=?
        )
    `, tableName).Scan(&exists)
	if err != nil {
		return false, errFactory.WithData(ErrSchemaValidationFailed, struct {
			Phase string
			Table string
			Error string
		}{
			Phase: "check_table_exists",
			Table: tableName,
			Error: err.Error(),
		})
	}
	return exists, nil
}

// SQL getters for consistent schema usage
func GetCreateTablesSQL() string {
	return createTablesSQL
}

// GetInsertMetricSQL returns the SQL to insert a metric
func GetInsertMetricSQL() string {
	return insertMetricsSQL
}
