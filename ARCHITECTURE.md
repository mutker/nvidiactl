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
│   │   ├── repository.go   # Data access implementation
│   │   ├── schema.go       # Data structure definitions
│   │   └── utils.go        # (Optional) Domain-specific utilities
│   ├── config/             # Config infrastructure
│   ├── errors/             # Error infrastructure
│   └── logger/             # Logging infrastructure
└── pkg/
```

### Package Naming

1. **Domain Packages**: Named after their core domain concept (e.g., gpu, telemetry)
2. **Infrastructure Packages**: Named after their cross-cutting concern (e.g., config, errors, logger)
3. **Command Packages**: Named after the executable they produce (e.g., nvidiactl)

### File Naming Conventions

1. **interface.go**: Contains all public interfaces and domain types
2. **{domain}.go**: Main domain implementation file, named after the package (e.g., telemetry.go, gpu.go)
3. **repository.go**: Data access interface and implementation
4. **config.go**: Domain configuration
5. **errors.go**: Domain-specific error codes
6. **schema.go**: Data structure definitions (if applicable)
7. **{file}_test.go**: Tests for the corresponding implementation file

### File Responsibilities

#### {domain}.go (e.g., telemetry.go, gpu.go)
- Primary domain implementation
- Implements interfaces defined in interface.go
- Contains core domain logic
- Named after the package for clear ownership

#### interface.go
- Public domain interfaces
- Domain types and value objects
- No implementation details

#### repository.go
- Data access interface
- Storage implementation
- Schema management

#### config.go
- Domain-specific configuration
- Configuration validation
- Default values

#### errors.go
- Domain-specific error codes
- Error constructors
- Error handling utilities

#### schema.go
- Data structure definitions
- Schema initialization
- Storage format specifications

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

Domain-specific packages (e.g., `internal/gpu`, `internal/telemetry`):
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

## Interface Design

Interfaces should be:
1. Defined at package boundaries
2. Focused on behavior, not implementation
3. Small and cohesive
4. Consumer-driven

Example:
```go
// internal/gpu/interface.go
type Controller interface {
    // Methods should describe behavior
    MonitorTemperature() (<-chan Temperature, error)
    SetFanSpeed(speed int) error
    GetCurrentState() (State, error)
}
```

## Error Handling

Error flow should:
1. Start specific in domains
2. Add context while bubbling up
3. Maintain technical details for logging
4. Present user-friendly messages at top level

Example:
```go
// Domain level
if err := gpu.SetFanSpeed(speed); err != nil {
    return errors.Wrap(ErrSetFanSpeed, "failed to adjust cooling")
}

// Top level
if err := controller.AdjustCooling(); err != nil {
    log.Error().Err(err).Msg("Technical details here")
    return fmt.Errorf("Failed to optimize GPU performance: %v", err)
}
```

## Dependencies

Dependencies should:
1. Flow through interfaces
2. Be injected from top level
3. Be minimal and explicit
4. Follow the dependency inversion principle

Example:
```go
type GPUController struct {
    device    DeviceController
    monitor   TempMonitor
    telemetry TelemetryCollector
}

func NewGPUController(deps Dependencies) *GPUController {
    return &GPUController{
        device:    deps.Device,
        monitor:   deps.Monitor,
        telemetry: deps.Telemetry,
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
