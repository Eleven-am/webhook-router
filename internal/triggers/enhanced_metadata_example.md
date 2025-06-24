# Enhanced Metadata Implementation Examples

This document shows how the enhanced metadata is populated and used across the system.

## Trigger Manager Integration

The trigger manager now creates rich metadata for all trigger executions:

```go
// In manager.go - wrapHandlerWithExecutionLogging
metadata := pipeline.NewMetadata(userID, *dbTrigger.PipelineID, executionLog.ID)
metadata.SetTriggerInfo(event.TriggerID, event.Type)

// Set source information based on trigger type
sourceMetadata := make(map[string]interface{})
if event.Headers != nil {
    sourceMetadata["headers"] = event.Headers
}
if event.Source != nil {
    sourceMetadata["source_name"] = event.Source.Name
    sourceMetadata["source_type"] = event.Source.Type
}

metadata.SetSourceInfo(event.Type, event.ID, sourceMetadata)

// Set environment and instance info
metadata.SetEnvironment(
    os.Getenv("ENVIRONMENT"),
    os.Getenv("REGION"), 
    os.Getenv("HOSTNAME"),
)

// Set trace ID if available
if traceID, ok := event.Headers["X-Trace-ID"]; ok {
    metadata.SetTracing(traceID, "", "", "")
}
```

## HTTP Trigger Enhanced Data

The HTTP trigger can now pass richer information in the TriggerEvent:

```go
// In HTTP trigger HandleHTTPRequest
event := &triggers.TriggerEvent{
    ID:          utils.GenerateEventID("http", t.config.ID),
    TriggerID:   t.config.ID,
    TriggerName: t.config.Name,
    Type:        "http",
    Timestamp:   time.Now(),
    Data: map[string]interface{}{
        "method":        r.Method,
        "path":          r.URL.Path,
        "query":         r.URL.RawQuery,
        "body":          string(body),
        "remote_addr":   r.RemoteAddr,
        "user_agent":    r.UserAgent(),
        "content_type":  r.Header.Get("Content-Type"),
        "content_length": r.ContentLength,
        "host":          r.Host,
        "scheme":        getScheme(r),
        "protocol":      r.Proto,
    },
    Headers: t.extractHeaders(r),
    Source: triggers.TriggerSource{
        Type:     "http",
        Name:     t.config.Name,
        Endpoint: t.config.Path,
        Metadata: map[string]interface{}{
            "method": r.Method,
            "path":   r.URL.Path,
        },
    },
}
```

## Broker Trigger Enhanced Data

```go
// In Broker trigger handleMessage
event := &triggers.TriggerEvent{
    ID:          utils.GenerateEventID("broker", t.config.ID),
    TriggerID:   t.config.ID,
    TriggerName: t.config.Name,
    Type:        "broker",
    Timestamp:   time.Now(),
    Data:        processedData,
    Headers:     msg.Headers,
    Source: triggers.TriggerSource{
        Type:     "broker",
        Name:     t.config.Name,
        Endpoint: t.config.BrokerType,
        Metadata: map[string]interface{}{
            "broker_type": t.config.BrokerType,
            "topic":       msg.Topic,
            "partition":   msg.Partition,
            "offset":      msg.Offset,
            "message_id":  msg.MessageID,
        },
    },
}
```

## Using Enhanced Metadata in Pipelines

### Example 1: Conditional Processing Based on Trigger Type

```json
{
  "id": "route-by-source",
  "type": "choice",
  "choices": [
    {
      "when": "${_metadata.trigger_type == 'http' && _metadata.source.metadata.method == 'POST'}",
      "execute": [
        {
          "id": "process-http-post",
          "type": "transform",
          "target": "result",
          "action": "{type: 'http_post', path: _metadata.source.metadata.path}"
        }
      ]
    },
    {
      "when": "${_metadata.trigger_type == 'broker' && _metadata.source.metadata.broker_type == 'kafka'}",
      "execute": [
        {
          "id": "process-kafka",
          "type": "transform",
          "target": "result",
          "action": "{type: 'kafka', topic: _metadata.source.metadata.topic, partition: _metadata.source.metadata.partition}"
        }
      ]
    },
    {
      "when": "${_metadata.trigger_type == 'schedule'}",
      "execute": [
        {
          "id": "process-scheduled",
          "type": "transform",
          "target": "result",
          "action": "{type: 'scheduled', cron: _metadata.source.metadata.cron}"
        }
      ]
    }
  ]
}
```

### Example 2: Audit Logging with Full Context

```json
{
  "id": "audit-log",
  "type": "http",
  "action": {
    "method": "POST",
    "url": "https://audit.example.com/log",
    "body": {
      "execution_id": "${_metadata.execution_id}",
      "user_id": "${_metadata.user_id}",
      "pipeline_id": "${_metadata.pipeline_id}",
      "trigger": {
        "id": "${_metadata.trigger_id}",
        "type": "${_metadata.trigger_type}"
      },
      "source": "${_metadata.source}",
      "environment": "${_metadata.environment}",
      "trace_id": "${_metadata.trace_id}",
      "start_time": "${_metadata.start_time}",
      "data": "${data}"
    }
  }
}
```

### Example 3: Environment-Based Configuration

```json
{
  "id": "select-api-endpoint",
  "type": "transform",
  "target": "api_url",
  "action": "${_metadata.environment == 'production' ? 'https://api.example.com' : 'https://api-staging.example.com'}"
}
```

### Example 4: Distributed Tracing Integration

```json
{
  "id": "call-downstream-service",
  "type": "http",
  "action": {
    "method": "POST",
    "url": "${api_url}/process",
    "headers": {
      "X-Trace-ID": "${_metadata.trace_id}",
      "X-Parent-Span-ID": "${_metadata.span_id}",
      "X-Correlation-ID": "${_metadata.correlation_id}"
    },
    "body": "${data}"
  }
}
```

### Example 5: Priority-Based Processing

```json
{
  "id": "set-processing-queue",
  "type": "choice",
  "choices": [
    {
      "when": "${_metadata.priority == 'high'}",
      "execute": [
        {
          "id": "high-priority-queue",
          "type": "publish",
          "broker": "kafka",
          "topic": "high-priority-processing"
        }
      ]
    },
    {
      "when": "${_metadata.priority == 'low'}",
      "execute": [
        {
          "id": "batch-queue",
          "type": "publish",
          "broker": "kafka",
          "topic": "batch-processing"
        }
      ]
    }
  ],
  "default": [
    {
      "id": "normal-queue",
      "type": "publish",
      "broker": "kafka",
      "topic": "normal-processing"
    }
  ]
}
```

## Benefits of Enhanced Metadata

1. **Complete Audit Trail**: Every execution has full context about its origin
2. **Conditional Processing**: Pipelines can behave differently based on source
3. **Distributed Tracing**: Built-in support for trace propagation
4. **Environment Awareness**: Easy to build environment-specific logic
5. **Performance Analysis**: Can analyze performance by trigger type, source, etc.
6. **Debugging**: Rich context makes debugging much easier
7. **Multi-tenancy**: User isolation is built into the metadata structure