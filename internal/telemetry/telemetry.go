package telemetry

import (
	"context"

	"codeberg.org/mutker/nvidiactl/internal/errors"
	"codeberg.org/mutker/nvidiactl/internal/logger"
)

type service struct {
	repo Repository
	cfg  Config
}

func NewService(cfg Config) (Collector, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(ErrInvalidConfig, err)
	}

	repo, err := NewRepository(cfg)
	if err != nil {
		return nil, err // Already wrapped with domain error
	}

	return &service{
		repo: repo,
		cfg:  cfg,
	}, nil
}

func (s *service) Record(ctx context.Context, snapshot *MetricsSnapshot) error {
	if snapshot == nil {
		return errors.New(ErrInvalidMetrics)
	}

	select {
	case <-ctx.Done():
		return errors.Wrap(ErrOperationTimeout, ctx.Err())
	default:
		if err := s.repo.Store(ctx, snapshot); err != nil {
			return errors.Wrap(ErrMetricsCollection, err)
		}

		logger.Debug().Msg("Telemetry metrics recorded successfully")
		return nil
	}
}

func (s *service) Close() error {
	if err := s.repo.Close(); err != nil {
		return errors.Wrap(ErrServiceShutdown, err)
	}
	return nil
}
