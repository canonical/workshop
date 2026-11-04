---
applyTo: "**/*.go"
description: Go-specific coding conventions for Workshop.
---

# Go Code Instructions

## Error Messages

**Format**: `what was attempted: why it went wrong`
- Start lowercase
- No trailing punctuation
- Specific, actionable context

**Example**:
```go
// Good
return fmt.Errorf("cannot launch workshop %q: container already exists", name)

// Avoid
return errors.New("Launch failed.")
```

## Error Handling Pattern

Prefer inline checks:
```go
// Preferred
if err := f(); err != nil {
    return err
}

// For multiple returns
val, err := f()
if err != nil {
    return err
}
```

## Code Organization

- **Early returns**: Avoid nested conditions
- **Coupling**: Keep related code adjacent (e.g., test data near test)
- **Variable declaration**: Initialize with declaration
- **Symmetries**: Handle identical operations uniformly

## Testing

- Unit tests: `*_test.go` files adjacent to implementation
- Use `internal/testutil/` helpers for common patterns
- Spread tests for integration scenarios

## Gold Standard Examples

- Error handling: [`client/client.go`](../../client/client.go) lines 50-80
- Interface implementation: [`internal/workshop/workshop.go`](../../internal/workshop/workshop.go)
- CLI command structure: [`cmd/workshop/launch.go`](../../cmd/workshop/launch.go)
