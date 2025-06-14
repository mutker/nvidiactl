# Architecture

## Overview

This document describes the architectural patterns and design principles, following a domain-driven design approach with clear boundaries and responsibilities between packages.

## Core Principles

1. **Domain Isolation**: Each domain package owns its specific workflow and implementation details
2. **Interface-Driven Design**: Dependencies flow through interfaces, not concrete implementations
3. **Clear Boundaries**: Packages have well-defined responsibilities and interaction patterns
4. **Top-Level Coordination**: High-level packages coordinate workflows, low-level packages implement domain logic
5. **Error Management**: Errors bubble up with rich context while maintaining domain separation

## Package Structure

```
nvidiactl/
├── cmd/
│   └── nvidiactl/          # Application entry point and workflow coordination
├── internal/
│   ├── domain1/            # Domain-specific package
│   │   ├── domain1.go      # Internal implementation of interfaces
│   │   ├── config.go       # Domain configuration
│   │   ├── errors.go       # Domain-specific errors
│   │   ├── interface.go    # Public domain interfaces and types
│   │   ├── repository.go   # (Optional) Data access implementation
│   │   ├── schema.go       # (Optional) Data structure definitions
│   │   └── utils.go        # (Optional) Domain-specific utilities
│   ├── config/             # Config infrastructure
│   ├── errors/             # Error infrastructure
│   └── logger/             # Logging infrastructure
└── pkg/
```

### Package Naming

1. **Domain Packages**: Named after their core domain concept (e.g., gpu, metrics)
2. **Infrastructure Packages**: Named after their cross-cutting concern (e.g., config, errors, logger)
3. **Command Packages**: Named after the executable they produce (e.g., nvidiactl)

### File Naming Conventions

1. **interface.go**: Contains all public interfaces and domain types
2. **{domain}.go**: Main domain implementation file, named after the package (e.g., metrics.go, gpu.go)
3. **repository.go**: Data access interface and implementation
4. **config.go**: Domain configuration
5. **errors.go**: Domain-specific error codes
6. **schema.go**: Data structure definitions (if applicable)
7. **{file}\_test.go**: Tests for the corresponding implementation file

### File Responsibilities

#### {domain}.go (e.g., metrics.go, gpu.go)

- Primary domain implementation
- Implements interfaces defined in interface.go
- Contains core domain logic
- Named after the package for clear ownership

#### interface.go

- Public domain interfaces
- Domain types and value objects
- No implementation details

#### config.go

- Domain-specific configuration
- Configuration validation
- Default values

#### errors.go

- Domain-specific error codes
- Error constructors
- Error handling utilities

### Package Dependencies

1. **Domain Package Dependencies**:

   - May depend on infrastructure packages (errors, logger, config)
   - Should not depend on other domain packages
   - Should expose clear interfaces for other packages to consume

2. **Infrastructure Package Dependencies**:

   - Should not depend on domain packages
   - May depend on other infrastructure packages
   - Should provide domain-agnostic functionality

3. **Command Package Dependencies**:
   - May depend on both domain and infrastructure packages
   - Responsible for wiring dependencies together
   - Should not expose any types to other packages

## Package Responsibilities

### cmd/nvidiactl

The main package is responsible for:

- Application entry point and lifecycle management
- Command-line interface and user interaction
- Workflow coordination between domains
- Error presentation and user feedback
- Domain package integration and dependency injection

Key principles:

- No direct domain logic implementation
- Coordinates workflow through domain interfaces
- Handles user interaction and feedback
- Manages application lifecycle

### internal/errors

Central error management package:

- Defines error types and codes
- Provides error wrapping and context
- Maintains error hierarchy
- Separates technical details from user messages

Key principles:

- Domain packages define and return rich errors
- Technical details flow to logs
- User-friendly messages flow to top level
- Maintains error context across domain boundaries

### internal/config

Configuration management package:

- Handles configuration loading and validation
- Manages environment variables
- Provides typed configuration access
- Validates configuration integrity

Key principles:

- Configuration flows through dependency injection
- Domain packages receive only relevant configuration
- Centralized validation and defaults
- Environment-aware configuration handling

### Domain Packages

Domain-specific packages (e.g., `internal/gpu`, `internal/metrics`):

- Own their domain workflow implementation
- Define clear public interfaces
- Maintain internal state and logic
- Handle domain-specific errors

Key principles:

- Clear separation of concerns
- Public interfaces for external interaction
- Internal implementation details hidden
- Domain-specific error handling
- Self-contained business logic

#### Schema Management (internal/metrics)

The metrics package implements a versioned schema approach:

1. **Schema Version Control**:

   - Schema definition is the single source of truth
   - Any schema change requires version increment
   - Version tracking in schema_versions table
   - Automatic backup before schema changes

2. **Schema Files Organization**:

   ```
   metrics/
   ├── schema.go      # Schema definition, version, and SQL
   └── migration.go   # Version check and backup handling
   ```

3. **Version Management Flow**:

   - Check current schema version on startup
   - If version mismatch:
     1. Create timestamped backup
     2. Drop existing tables
     3. Create new schema with new version
     4. Log backup location for reference

4. **Backup Strategy**:

   - Backups stored in `/var/lib/nvidiactl/backups`
   - Naming: `metrics_v{version}_{timestamp}.db`
   - Uses SQLite VACUUM for safe backup
   - Filesystem-based backup management

5. **Schema Safety Principles**:
   - Schema changes require version increment
   - Automatic backup before changes
   - Clean separation of versioning and data
   - Safe initialization and cleanup

## Error Handling

Error flow should:

1. Start specific in domains using domain error codes
2. Use error factory pattern consistently
3. Add context while bubbling up
4. Maintain technical details for logging

### Error Code Hierarchy

1. **Common Infrastructure Errors**:

   - Defined in central errors package
   - Represent cross-cutting concerns
   - Used for truly common scenarios (e.g., initialization, timeouts)
   - Example: `ErrTimeout`, `ErrInitFailed`, `ErrInvalidConfig`

2. **Domain-Specific Errors**:
   - Defined within domain packages
   - Represent domain-specific failures
   - May use common errors where appropriate
   - Should maintain clear domain context
   - Example: `metrics.ErrSchemaValidationFailed`, `gpu.ErrFanControlFailed`

### Error Context

Errors should carry appropriate context using the error factory methods:

1. `New`: Create new domain error
2. `Wrap`: Wrap underlying error with domain context
3. `WithMessage`: Add descriptive message
4. `WithData`: Attach structured data

Example:

```go
// Domain level
errFactory := errors.New()
if err := device.SetFanSpeed(speed); !IsNVMLSuccess(ret) {
    return errFactory.Wrap(ErrSetFanSpeed, newNVMLError(ret))
}

// With structured data
return errFactory.WithData(ErrSchemaValidationFailed,
    struct {
        Table  string
        Column string
        Error  string
    }{
        Table:  "metrics",
        Column: "timestamp",
        Error:  "type mismatch",
    })

// Top level error handling
if err := controller.AdjustCooling(); err != nil {
    var domainErr errors.Error
    if !errors.As(err, &domainErr) {
        domainErr = errFactory.Wrap(errors.ErrMainLoop, err)
    }
    logger.ErrorWithCode(domainErr).Send()
}
```

### Error Design Principles

1. **Domain Separation**:

   - Each domain owns its error definitions
   - Domains may use common errors where appropriate
   - Error context remains domain-specific

2. **Error Factory Pattern**:

   - Consistent error creation through factory
   - Rich error context and metadata
   - Type-safe error handling

3. **Error Flow**:

   - Technical details captured at source
   - Context added while bubbling up
   - User-friendly messages at top level

4. **Error Context**:
   - Structured data over string formatting
   - Clear error hierarchies
   - Rich debugging information

## Dependencies

Dependencies should:

1. Flow through interfaces, never concrete types.
2. Be explicitly injected from the top level (e.g., `main.go`). The use of global variables or singletons for dependencies like loggers or configuration providers is strictly forbidden.
3. Be minimal and explicit. A component should only receive the dependencies it directly needs.
4. Follow the dependency inversion principle, where high-level modules do not depend on low-level modules, but both depend on abstractions (interfaces).

Example:

```go
type GPUController struct {
    device    DeviceController
    monitor   TempMonitor
    metrics MetricsCollector
}

func NewGPUController(deps Dependencies) *GPUController {
    return &GPUController{
        device:    deps.Device,
        monitor:   deps.Monitor,
        metrics: deps.Metrics,
    }
}
```

## Testing

Testing strategy should:

1. Test domain logic in isolation
2. Use interfaces for mocking
3. Test integration at top level
4. Maintain test hierarchy matching package structure

Example:

```go
// Domain test
func TestGPUController_SetFanSpeed(t *testing.T) {
    // Test specific domain logic
}

// Integration test in cmd/nvidiactl
func TestCoolingWorkflow(t *testing.T) {
    // Test workflow coordination
}
```

## Contributing Guidelines

When contributing:

1. Maintain package boundaries
2. Add interfaces for new dependencies
3. Follow error handling patterns
4. Update tests at appropriate levels
5. Document public interfaces
6. Keep domains focused and cohesive

## Version Control

Package changes should:

1. Be atomic and focused
2. Include relevant test updates
3. Maintain backward compatibility
4. Document interface changes
5. Update relevant documentation

## Concurrency

Safe concurrent programming is critical to the stability of this application. The following principles must be adhered to:

1.  **Protect Shared State**: Any access to shared state that can be read and written by multiple goroutines must be protected by a mutex (`sync.Mutex` or `sync.RWMutex`).
2.  **Use Appropriate Locks**: Use `sync.RWMutex` when a resource is read much more often than it is written to, allowing for concurrent reads. Use `sync.Mutex` for exclusive access.
3.  **Consistent Locking**: Ensure that all access paths to a shared resource are protected by the same lock.
4.  **Avoid Global State**: Minimize the use of global variables. When unavoidable, ensure they are protected by mutexes if accessed concurrently.
