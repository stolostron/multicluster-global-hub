# Global Hub Version Mechanism

## Overview

The Global Hub project implements a version mechanism to ensure data consistency and ordering between Agent and Manager components. This mechanism provides:

- Ordered data processing
- Prevention of duplicate processing of old data
- Dependency relationship management
- Resync capability after agent restarts

## Core Components

### Version Structure

The version system uses a two-level versioning approach:

```go
type Version struct {
    Generation uint64 `json:"Generation"`  // Incremented when sending to hub
    Value      uint64 `json:"Value"`       // Incremented when bundle is updated
}
```

**Key Fields:**

- `Generation`: Tracks sending batches, used to detect agent restarts
- `Value`: Tracks content changes, ensures data freshness

**Version Format**: `Generation.Value` (e.g., "2.15")

## How It Works

### Agent Side

**Location**: `pkg/bundle/version/version.go`, `agent/pkg/status/generic/generic_emitter.go`

1. **Content Updates**: When bundle content changes, call `version.Incr()` to increment `Value`
2. **Event Sending**: When sending events to hub, call `version.Next()` to increment `Generation`
3. **Version Embedding**: Version is embedded in CloudEvent extensions as `extversion`

**Single Mode Note**: In single event mode, instead of batching multiple objects into one CloudEvent, each object is sent as a separate CloudEvent. The version remains the same for these individual events that were originally part of the same batch. This requires careful version handling to prevent race conditions.

### Manager Side

**Location**: `manager/pkg/status/conflator/element_*.go`

The Manager uses **Conflator** components to handle version validation:

1. **Version Validation**: Process events with versions newer than or equal to `lastProcessedVersion`
2. **Agent Restart Detection**: When `Generation=0`, reset state for resync
3. **Dependency Processing**: Ensure dependent events are processed in correct order

**Race Condition Prevention**: The version check uses `NewerThanOrEqual()` with simplified logic that compares both generation and value fields simultaneously (`v.Generation >= other.Generation && v.Value >= other.Value`), ensuring events with equal versions are processed and preventing event loss in single mode.

## Key Design Principles

### 1. Agent Restart Detection

- **Initial Version**: Agent starts with version `0.0`, first event is `0.1`
- **Manager Response**: When receiving `Generation=0`, Manager resets conflation state
- **Resync**: Ensures complete state synchronization after restarts

### 2. Two-Level Version Control

- **Generation**: Prevents processing stale events from previous agent sessions
- **Value**: Ensures only the latest content updates are processed

### 3. Event Ordering

- **Delta Events**: Processed immediately if version is newer
- **Complete Events**: May be queued until dependencies are satisfied
- **Dependencies**: Some events depend on others being processed first

### 4. Memory Management

- **Overflow Protection**: Versions reset when approaching `MaxUint64`
- **Efficient Storage**: Only store necessary version metadata
- **State Cleanup**: Clean up processed events to prevent memory leaks

## Usage Examples

### Agent Side - Sending Events

```go
// Update content
emitter.PostUpdate()  // Calls version.Incr()

// Send to hub
emitter.PostSend()    // Calls version.Next()
```

### Manager Side - Receiving Events

```go
func (h *handler) handleEvent(ctx context.Context, evt *cloudevents.Event) error {
    version := evt.Extensions()[eventversion.ExtVersion]
    // Version validation happens in conflator before reaching handler
    return h.processEvent(evt)
}
```

### Dependency Management

Some events have dependencies and include `extdependencyversion`:

```go
e.SetExtension(eventversion.ExtDependencyVersion, dependencyVersion.String())
```

## Benefits

- **Consistency**: Prevents data races and duplicate processing
- **Reliability**: Handles agent restarts gracefully
- **Performance**: Efficient version comparison and validation
- **Scalability**: Supports multiple agents with independent version sequences

## Troubleshooting

### Events Being Dropped

- **Cause**: Event version is older than `lastProcessedVersion`
- **Solution**: Check version progression and agent restart handling

### Data Inconsistency

- **Cause**: Version validation not working correctly
- **Solution**: Verify `Generation=0` handling and conflator configuration

### Performance Issues

- **Cause**: Excessive version comparisons or long dependency chains
- **Solution**: Optimize dependency relationships and conflation logic
