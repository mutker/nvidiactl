package metrics

import "codeberg.org/mutker/nvidiactl/internal/errors"

const (
	// File system permissions and paths
	defaultDirPerm  = 0o755
	defaultFilePerm = 0o644
	defaultDBPath   = "/var/lib/nvidiactl/metrics.db"

	// Batching defaults
	defaultBatchSize    = 100
	defaultBatchTimeout = 5
)

type Config struct {
	DBPath          string
	SchemaVersion   int
	BackupOnMigrate bool
	Enabled         bool
	BatchSize       int
	BatchTimeout    int
}

func DefaultConfig() Config {
	return Config{
		DBPath:       defaultDBPath,
		Enabled:      false, // Disabled by default
		BatchSize:    defaultBatchSize,
		BatchTimeout: defaultBatchTimeout,
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
