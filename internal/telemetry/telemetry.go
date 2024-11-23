package telemetry

import (
	"context"

	"codeberg.org/mutker/nvidiactl/internal/errors"
)

type service struct {
	repo Repository
	cfg  Config
}

func NewService(cfg Config) (Collector, error) {
	errFactory := errors.New()

	if err := cfg.Validate(); err != nil {
		return nil, errFactory.Wrap(ErrInvalidConfig, err)
	}

	repo, err := NewRepository(cfg)
	if err != nil {
		return nil, err // Already wrapped with appropriate error
	}

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
