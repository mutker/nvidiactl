package telemetry

import "codeberg.org/mutker/nvidiactl/internal/errors"

const (
	defaultDirPerm = 0o755
	defaultDBPath  = "/var/lib/nvidiactl/telemetry.db"
)

type Config struct {
	DBPath string
}

func DefaultConfig() Config {
	return Config{
		DBPath: defaultDBPath,
	}
}

func (c Config) Validate() error {
	errFactory := errors.New()
	if c.DBPath == "" {
		return errFactory.New(ErrInvalidDBPath)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
