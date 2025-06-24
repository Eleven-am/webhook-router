# Pipeline Metadata Injection

This document explains how metadata (user ID and pipeline ID) is injected into pipeline execution context.

## How It Works

1. **Trigger Manager** calls `ExecutePipelineWithMetadata` when executing a pipeline:
```go
metadata := map[string]interface{}{
    "user_id":     userID,
    "pipeline_id": pipelineID,
}
result, err := pipelineEngine.ExecutePipelineWithMetadata(ctx, pipelineID, data, metadata)
```

2. **Pipeline Engine** injects metadata into the context:
```go
// Metadata is stored in the input data as a single object
inputData["_metadata"] = map[string]interface{}{
    "user_id":     metadata.UserID,
    "pipeline_id": metadata.PipelineID,
}

// Also injected into cache stages
stages.InjectMetadata(registry, metadata.PipelineID, metadata.UserID)
```

3. **Cache Stages** use the injected metadata:
```go
// Cache stage checks for metadata
if metadata, ok := runCtx.Get("_metadata"); ok {
    if metaMap, ok := metadata.(map[string]interface{}); ok {
        e.userID = fmt.Sprint(metaMap["user_id"])
        e.pipelineID = fmt.Sprint(metaMap["pipeline_id"])
    }
}

// Generate cache key with user scope
cacheKey := fmt.Sprintf("user:%s:pipeline:%s:stage:%s:key:%s",
    e.userID,
    e.pipelineID,
    stage.ID,
    hashedKey,
)
```

## Available in Pipeline Context

Stages can access metadata through the context:

```go
// Get metadata object
if metadata, ok := ctx.Get("_metadata"); ok {
    meta := metadata.(map[string]interface{})
    userID := meta["user_id"].(string)
    pipelineID := meta["pipeline_id"].(string)
}
```

## Usage in Stages

### Example: Audit Stage
```json
{
  "id": "audit-log",
  "type": "transform",
  "target": "auditEntry",
  "expression": "{
    user: _metadata.user_id,
    pipeline: _metadata.pipeline_id,
    action: 'data_processed',
    timestamp: now()
  }"
}
```

### Example: User-Scoped HTTP Request
```json
{
  "id": "fetch-user-data",
  "type": "http",
  "action": {
    "method": "GET",
    "url": "https://api.example.com/users/${_metadata.user_id}/profile",
    "headers": {
      "X-Pipeline-ID": "${_metadata.pipeline_id}"
    }
  }
}
```

## Benefits

1. **Automatic User Isolation**: Cache entries are automatically scoped to users
2. **Audit Trail**: Every pipeline execution can track who initiated it
3. **Access Control**: Stages can enforce user-specific permissions
4. **Debugging**: Easier to trace issues back to specific users/pipelines