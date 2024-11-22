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
│   │   ├── interface.go    # Public domain interfaces
│   │   ├── domain1.go      # Internal domain implementation
│   │   └── errors.go       # Domain-specific errors
│   ├── config/             # Configuration management
│   ├── errors/             # Central error definitions
│   └── logger/             # Logging infrastructure
└── pkg/                    # Public API packages (if any)
```

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
