package telemetry

import "codeberg.org/mutker/nvidiactl/internal/errors"

const (
	// Configuration Errors
	ErrInvalidConfig = errors.ErrorCode("telemetry_invalid_config")
	ErrInvalidDBPath = errors.ErrorCode("telemetry_invalid_db_path")

	// Collection Errors
	ErrMetricsCollection = errors.ErrorCode("telemetry_metrics_collection_failed")
	ErrInvalidMetrics    = errors.ErrorCode("telemetry_invalid_metrics")

	// Storage Errors
	ErrStorageAccess    = errors.ErrorCode("telemetry_storage_access_failed")
	ErrStorageInit      = errors.ErrorCode("telemetry_storage_init_failed")
	ErrStorageClose     = errors.ErrorCode("telemetry_storage_close_failed")
	ErrSchemaInitFailed = errors.ErrorCode("telemetry_schema_init_failed")

	// Operation Errors
	ErrOperationTimeout = errors.ErrorCode("telemetry_operation_timeout")
	ErrServiceShutdown  = errors.ErrorCode("telemetry_service_shutdown_failed")
)
