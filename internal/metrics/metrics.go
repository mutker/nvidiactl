package metrics

import (
	"context"

	"codeberg.org/mutker/nvidiactl/internal/errors"
	"codeberg.org/mutker/nvidiactl/internal/logger"
)

type service struct {
	repo MetricsRepository
	cfg  Config
}

// No-op implementation
type noopMetricsCollector struct{}

func NewService(cfg Config) (MetricsCollector, error) {
	errFactory := errors.New()

	if err := cfg.Validate(); err != nil {
		return nil, errFactory.Wrap(ErrInvalidConfig, err)
	}

	// If metrics is disabled, return a no-op collector
	if !cfg.Enabled {
		logger.Debug().Msg("Metrics collection disabled, using no-op collector")
		return &noopMetricsCollector{}, nil
	}

	// Remove reference to undefined removeOldDatabase
	repo, err := NewRepository(cfg)
	if err != nil {
		logger.Debug().Err(err).Msg("Failed to create metrics repository")
		return nil, err
	}

	logger.Debug().
		Str("db_path", cfg.DBPath).
		Bool("enabled", cfg.Enabled).
		Msg("Metrics service initialized successfully")

	return &service{
		repo: repo,
		cfg:  cfg,
	}, nil
}

func (s *service) Record(ctx context.Context, snapshot *MetricsSnapshot) error {
	errFactory := errors.New()

	if snapshot == nil {
		return errFactory.New(ErrInvalidMetrics)
	}

	select {
	case <-ctx.Done():
		return errFactory.Wrap(ErrOperationTimeout, ctx.Err())
	default:
		if err := s.repo.Record(snapshot); err != nil {
			return errFactory.Wrap(ErrMetricsCollection, err)
		}
	}

	return nil
}

func (s *service) Close() error {
	errFactory := errors.New()

	if err := s.repo.Close(); err != nil {
		return errFactory.Wrap(ErrServiceShutdown, err)
	}
	return nil
}

// No-op implementation
func (*noopMetricsCollector) Record(_ context.Context, _ *MetricsSnapshot) error {
	return nil
}

func (*noopMetricsCollector) Close() error {
	return nil
}
