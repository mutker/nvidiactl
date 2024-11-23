package metrics

import "codeberg.org/mutker/nvidiactl/internal/errors"

const (
	// File system permissions and paths
	defaultDirPerm  = 0o755
	defaultFilePerm = 0o644
	defaultDBPath   = "/var/lib/nvidiactl/metrics.db"
)

type Config struct {
	DBPath          string
	SchemaVersion   int
	BackupOnMigrate bool
	Enabled         bool
}

func DefaultConfig() Config {
	return Config{
		DBPath:  defaultDBPath,
		Enabled: false, // Disabled by default
	}
}

func (c Config) Validate() error {
	errFactory := errors.New()

	// Only validate DBPath if metrics is enabled
	if c.Enabled && c.DBPath == "" {
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
