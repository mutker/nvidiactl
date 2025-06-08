package metrics

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"codeberg.org/mutker/nvidiactl/internal/errors"
	"codeberg.org/mutker/nvidiactl/internal/logger"
)

const backupDir = "/var/lib/nvidiactl/backups"

func backupDatabase(db *sql.DB, version int, log logger.Logger) (string, error) {
	errFactory := errors.New()

	// Ensure backup directory exists
	if err := os.MkdirAll(backupDir, defaultDirPerm); err != nil {
		return "", errFactory.WithData(ErrSchemaInitFailed, struct {
			Phase string
			Path  string
			Error string
		}{
			Phase: "create_backup_dir",
			Path:  backupDir,
			Error: err.Error(),
		})
	}

	// Create backup filename with timestamp
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	backupPath := filepath.Join(backupDir,
		fmt.Sprintf("metrics_v%d_%s.db", version, timestamp))

	// VACUUM INTO requires no active transaction
	_, err := db.Exec(fmt.Sprintf("VACUUM INTO '%s'", backupPath))
	if err != nil {
		return "", errFactory.WithData(ErrSchemaInitFailed, struct {
			Phase string
			Path  string
			Error string
		}{
			Phase: "create_backup",
			Path:  backupPath,
			Error: err.Error(),
		})
	}

	log.Info().
		Str("path", backupPath).
		Int("version", version).
		Msg("Database backup created")

	return backupPath, nil
}

// ValidateAndUpdateSchema checks the schema version and recreates it if needed.
// If a schema exists but the version doesn't match, it creates a backup
// before recreating the schema.
func ValidateAndUpdateSchema(db *sql.DB, log logger.Logger) error {
	errFactory := errors.New()

	version, err := GetSchemaVersion(db)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get schema version")
		return errFactory.Wrap(ErrSchemaValidationFailed, err)
	}

	log.Debug().
		Int("version", version).
		Bool("init_db", version == 0).
		Msg("Current schema version")

	// New database or version mismatch
	if version == 0 || version != SchemaVersion {
		// If existing schema, backup first
		if version != 0 {
			backupPath, err := backupDatabase(db, version, log)
			if err != nil {
				return errFactory.WithData(ErrSchemaMigrationFailed, struct {
					Phase string
					Error string
					Path  string
				}{
					Phase: "backup",
					Error: err.Error(),
					Path:  backupPath,
				})
			}
		}

		// Drop existing tables and create new schema
		if err := dropTables(db, log); err != nil {
			return err
		}
		return InitSchema(db, log)
	}

	log.Debug().
		Int("version", version).
		Msg("Schema version is current")
	return nil
}

func dropTables(db *sql.DB, log logger.Logger) error {
	errFactory := errors.New()

	tx, err := db.Begin()
	if err != nil {
		return errFactory.Wrap(ErrSchemaMigrationFailed, err)
	}

	// Track transaction state
	committed := false
	defer func() {
		if !committed {
			if err := tx.Rollback(); err != nil {
				// Only log if it's not the "already committed" error
				if !errors.Is(err, sql.ErrTxDone) {
					log.Debug().Err(err).Msg("Failed to rollback drop tables")
				}
			}
		}
	}()

	tables := []string{"metrics", "schema_versions"}
	for _, table := range tables {
		if _, err := tx.Exec("DROP TABLE IF EXISTS " + table); err != nil {
			return errFactory.WithData(ErrSchemaMigrationFailed, struct {
				Phase string
				Table string
				Error string
			}{
				Phase: "drop_table",
				Table: table,
				Error: err.Error(),
			})
		}
	}

	if err := tx.Commit(); err != nil {
		return errFactory.WithData(ErrSchemaMigrationFailed, struct {
			Phase string
			Error string
		}{
			Phase: "commit_changes",
			Error: err.Error(),
		})
	}
	committed = true

	return nil
}
