package metrics

import "codeberg.org/mutker/nvidiactl/internal/errors"

const (
	// Configuration Errors
	ErrInvalidConfig = errors.ErrInvalidConfig
	ErrInvalidDBPath = errors.ErrorCode("metrics_invalid_db_path")

	// Schema Errors
	ErrSchemaInitFailed       = errors.ErrorCode("metrics_schema_init_failed")
	ErrSchemaValidationFailed = errors.ErrorCode("metrics_schema_validation_failed")
	ErrSchemaMigrationFailed  = errors.ErrorCode("metrics_schema_migration_failed")
	ErrTransactionFailed      = errors.ErrorCode("metrics_transaction_failed")

	// Storage Errors
	ErrStorageAccess = errors.ErrorCode("metrics_storage_access_failed")
	ErrStorageInit   = errors.ErrInitFailed
	ErrStorageClose  = errors.ErrShutdownFailed

	// Service Errors
	ErrServiceShutdown = errors.ErrShutdownFailed

	// Collection Errors
	ErrMetricsCollection = errors.ErrorCode("metrics_metrics_collection_failed")
	ErrInvalidMetrics    = errors.ErrorCode("metrics_invalid_metrics")

	// Operation Errors
	ErrOperationTimeout = errors.ErrTimeout
)
