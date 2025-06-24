# Trigger-Agnostic Metadata Examples

This document shows how different trigger types would populate the generic metadata structure.

## HTTP Trigger

```go
func (t *HTTPTrigger) createMetadata(r *http.Request, executionLog *storage.ExecutionLog) *pipeline.Metadata {
    metadata := pipeline.NewMetadata(
        executionLog.UserID,
        executionLog.PipelineID,
        executionLog.ID,
    )
    
    metadata.SetTriggerInfo(t.ID, "http")
    metadata.SetSourceInfo("http_request", r.URL.Path, map[string]interface{}{
        "method":      r.Method,
        "path":        r.URL.Path,
        "query":       r.URL.RawQuery,
        "remote_addr": r.RemoteAddr,
        "user_agent":  r.Header.Get("User-Agent"),
        "request_id":  r.Header.Get("X-Request-ID"),
    })
    
    return metadata
}
```

## Broker Trigger (Kafka/RabbitMQ/Redis)

```go
func (t *BrokerTrigger) createMetadata(msg *brokers.IncomingMessage, executionLog *storage.ExecutionLog) *pipeline.Metadata {
    metadata := pipeline.NewMetadata(
        executionLog.UserID,
        executionLog.PipelineID,
        executionLog.ID,
    )
    
    metadata.SetTriggerInfo(t.ID, "broker")
    
    sourceMetadata := map[string]interface{}{
        "broker_type": t.config.Type,
        "headers":     msg.Headers,
    }
    
    // Add broker-specific metadata
    switch t.config.Type {
    case "kafka":
        sourceMetadata["topic"] = msg.Topic
        sourceMetadata["partition"] = msg.Partition
        sourceMetadata["offset"] = msg.Offset
        sourceMetadata["key"] = msg.Key
    case "rabbitmq":
        sourceMetadata["exchange"] = msg.Exchange
        sourceMetadata["routing_key"] = msg.RoutingKey
        sourceMetadata["delivery_tag"] = msg.DeliveryTag
    case "redis":
        sourceMetadata["channel"] = msg.Channel
        sourceMetadata["pattern"] = msg.Pattern
    }
    
    identifier := fmt.Sprintf("%s:%s", t.config.Type, msg.MessageID)
    metadata.SetSourceInfo("message", identifier, sourceMetadata)
    
    return metadata
}
```

## Schedule Trigger

```go
func (t *ScheduleTrigger) createMetadata(executionLog *storage.ExecutionLog) *pipeline.Metadata {
    metadata := pipeline.NewMetadata(
        executionLog.UserID,
        executionLog.PipelineID,
        executionLog.ID,
    )
    
    metadata.SetTriggerInfo(t.ID, "schedule")
    
    now := time.Now()
    next := t.schedule.Next(now)
    
    metadata.SetSourceInfo("schedule", t.config.Name, map[string]interface{}{
        "cron":           t.config.Cron,
        "scheduled_time": now.Format(time.RFC3339),
        "next_run":       next.Format(time.RFC3339),
        "timezone":       t.config.Timezone,
    })
    
    return metadata
}
```

## Polling Trigger

```go
func (t *PollingTrigger) createMetadata(resource string, executionLog *storage.ExecutionLog) *pipeline.Metadata {
    metadata := pipeline.NewMetadata(
        executionLog.UserID,
        executionLog.PipelineID,
        executionLog.ID,
    )
    
    metadata.SetTriggerInfo(t.ID, "polling")
    metadata.SetSourceInfo("poll", resource, map[string]interface{}{
        "resource":      resource,
        "poll_interval": t.config.Interval,
        "last_checked":  t.lastChecked.Format(time.RFC3339),
        "changes_found": true,
    })
    
    return metadata
}
```

## CalDAV/CardDAV Trigger

```go
func (t *CalDAVTrigger) createMetadata(event *caldav.Event, executionLog *storage.ExecutionLog) *pipeline.Metadata {
    metadata := pipeline.NewMetadata(
        executionLog.UserID,
        executionLog.PipelineID,
        executionLog.ID,
    )
    
    metadata.SetTriggerInfo(t.ID, "caldav")
    metadata.SetSourceInfo("calendar_event", event.UID, map[string]interface{}{
        "calendar_url": t.config.URL,
        "event_uid":    event.UID,
        "event_type":   "created", // or "updated", "deleted"
        "summary":      event.Summary,
        "start_time":   event.StartTime,
    })
    
    return metadata
}
```

## IMAP Trigger

```go
func (t *IMAPTrigger) createMetadata(email *imap.Message, executionLog *storage.ExecutionLog) *pipeline.Metadata {
    metadata := pipeline.NewMetadata(
        executionLog.UserID,
        executionLog.PipelineID,
        executionLog.ID,
    )
    
    metadata.SetTriggerInfo(t.ID, "imap")
    metadata.SetSourceInfo("email", email.MessageID, map[string]interface{}{
        "server":     t.config.Server,
        "folder":     t.config.Folder,
        "message_id": email.MessageID,
        "subject":    email.Subject,
        "from":       email.From,
        "date":       email.Date,
    })
    
    return metadata
}
```

## Using Metadata in Pipeline Stages

Pipeline stages can access this metadata to make decisions:

```javascript
// In a transform stage expression:
{
    source_type: _metadata.source.type,
    is_scheduled: _metadata.trigger_type == "schedule",
    is_retry: _metadata.retry_count > 0,
    environment: _metadata.environment,
    
    // Conditional logic based on trigger type
    notification_channel: _metadata.trigger_type == "http" ? "webhook" : "email",
    
    // Use source metadata
    original_topic: _metadata.source.metadata.topic,
    request_method: _metadata.source.metadata.method
}
```

## Benefits of This Approach

1. **Trigger Agnostic**: Works for any trigger type without HTTP-specific assumptions
2. **Extensible**: Each trigger can add its own metadata without breaking others
3. **Debugging**: Easy to trace execution back to its source
4. **Conditional Logic**: Pipelines can behave differently based on trigger type
5. **Audit Trail**: Complete record of what initiated each execution
6. **Performance Analysis**: Can analyze performance by trigger type, source, etc.