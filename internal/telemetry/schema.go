package telemetry

import (
	"database/sql"

	"codeberg.org/mutker/nvidiactl/internal/errors"
)

func initSchema(db *sql.DB) error {
	errFactory := errors.New()

	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS metrics (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            timestamp TEXT NOT NULL,
            fan_speed_current INTEGER NOT NULL,
            fan_speed_target INTEGER NOT NULL,
            temp_current REAL NOT NULL,
            temp_average REAL NOT NULL,
            power_current INTEGER NOT NULL,
            power_target INTEGER NOT NULL,
            power_average REAL NOT NULL,
            auto_fan_control INTEGER NOT NULL,
            performance_mode INTEGER NOT NULL
        )
    `)
	if err != nil {
		return errFactory.Wrap(ErrSchemaInitFailed, err)
	}

	return nil
}
