package telemetry

import (
	"database/sql"

	"codeberg.org/mutker/nvidiactl/internal/errors"
)

func initSchema(db *sql.DB) error {
	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS telemetry (
            timestamp INTEGER PRIMARY KEY,
            fan_speed INTEGER,
            target_fan_speed INTEGER,
            temperature INTEGER,
            average_temperature INTEGER,
            power_limit INTEGER,
            target_power_limit INTEGER,
            average_power_limit INTEGER,
            auto_fan_control INTEGER,
            performance_mode INTEGER
        )
    `)
	if err != nil {
		return errors.Wrap(ErrSchemaInitFailed, err)
	}

	return nil
}
