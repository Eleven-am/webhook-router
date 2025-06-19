# Branch Stage

The Branch stage is a terminal pipeline stage that enables conditional routing to different inline pipelines based on filter conditions. It evaluates conditions in order and executes the first matching pipeline.

## Overview

- **Type**: `branch`
- **Purpose**: Route data to different processing paths based on conditions
- **Terminal**: Yes - this stage must be the last in its pipeline
- **Execution**: First match wins - only the first matching condition's pipeline is executed

## Configuration

```json
{
  "type": "branch",
  "conditions": [
    {
      "filter": {
        "field": "field_name",
        "operator": "comparison_operator",
        "value": "comparison_value"
      },
      "pipeline": [
        // Array of stage configurations
      ]
    }
  ]
}
```

### Filter Operators

The branch stage supports all filter operators:
- `eq`: Equals
- `ne`: Not equals
- `gt`: Greater than
- `lt`: Less than
- `gte`: Greater than or equal
- `lte`: Less than or equal
- `contains`: String contains
- `in`: Value in list
- `exists`: Field exists

## Examples

### 1. Route by Order Type

```json
{
  "stages": [
    {
      "type": "validate",
      "fields": {
        "type": "required",
        "amount": "required,numeric"
      }
    },
    {
      "type": "branch",
      "conditions": [
        {
          "filter": {
            "field": "type",
            "operator": "eq",
            "value": "premium"
          },
          "pipeline": [
            {
              "type": "json_transform",
              "operation": "merge",
              "merge_data": {
                "priority": "high",
                "expedited": true
              }
            },
            {
              "type": "webhook",
              "url": "https://api.example.com/premium-orders"
            }
          ]
        },
        {
          "filter": {
            "field": "amount",
            "operator": "gt",
            "value": 1000
          },
          "pipeline": [
            {
              "type": "json_transform",
              "operation": "merge",
              "merge_data": {
                "requires_approval": true
              }
            },
            {
              "type": "webhook",
              "url": "https://api.example.com/high-value-orders"
            }
          ]
        },
        {
          "filter": {
            "field": "type",
            "operator": "exists"
          },
          "pipeline": [
            {
              "type": "json_transform",
              "operation": "merge",
              "merge_data": {
                "status": "standard"
              }
            }
          ]
        }
      ]
    }
  ]
}
```

### 2. Error Handling Branch

```json
{
  "type": "branch",
  "conditions": [
    {
      "filter": {
        "field": "error",
        "operator": "exists"
      },
      "pipeline": [
        {
          "type": "json_transform",
          "operation": "merge",
          "merge_data": {
            "error_handled": true,
            "handled_at": "{{now}}"
          }
        },
        {
          "type": "dlq",
          "queue": "error-queue"
        }
      ]
    },
    {
      "filter": {
        "field": "success",
        "operator": "eq",
        "value": true
      },
      "pipeline": [
        {
          "type": "json_transform",
          "operation": "extract",
          "extract_fields": ["id", "result", "timestamp"]
        }
      ]
    }
  ]
}
```

### 3. Complex Processing with JavaScript

```json
{
  "type": "branch",
  "conditions": [
    {
      "filter": {
        "field": "process_type",
        "operator": "eq",
        "value": "transform"
      },
      "pipeline": [
        {
          "type": "js_transform",
          "script": "return {...input, calculated: input.value * 1.2};"
        },
        {
          "type": "filter",
          "conditions": [
            {"field": "calculated", "operator": "gt", "value": 100}
          ],
          "action": "include"
        },
        {
          "type": "json_transform",
          "operation": "merge",
          "merge_data": {
            "approved": true
          }
        }
      ]
    }
  ]
}
```

## Behavior

### Condition Evaluation

1. Conditions are evaluated in the order they appear
2. The first matching condition's pipeline is executed
3. Subsequent conditions are not evaluated after a match
4. If no conditions match, the workflow ends successfully

### Pipeline Execution

- Each inline pipeline is executed as a complete pipeline
- All stages in the pipeline are executed sequentially
- If any stage fails, the branch stage returns the failure
- The output of the inline pipeline becomes the output of the branch stage

## Use Cases

### 1. Request Routing
Route different types of webhooks to appropriate processing:
```json
{
  "filter": {"field": "webhook_type", "operator": "eq", "value": "github"},
  "pipeline": [/* GitHub-specific processing */]
}
```

### 2. Value-Based Processing
Apply different transformations based on data values:
```json
{
  "filter": {"field": "customer_tier", "operator": "eq", "value": "gold"},
  "pipeline": [/* Premium customer processing */]
}
```

### 3. Error Recovery
Handle errors with specific recovery pipelines:
```json
{
  "filter": {"field": "retry_count", "operator": "gt", "value": 3},
  "pipeline": [/* Dead letter queue routing */]
}
```

### 4. A/B Testing
Route traffic to different processing paths:
```json
{
  "filter": {"field": "experiment_group", "operator": "eq", "value": "A"},
  "pipeline": [/* Variant A processing */]
}
```

## Best Practices

1. **Order Matters**: Place more specific conditions before general ones
2. **Default Handling**: Consider adding a catch-all condition at the end
3. **Keep It Simple**: Avoid deeply nested pipelines within branches
4. **Performance**: First-match-wins means order affects performance
5. **Validation**: Validate data before branching to ensure fields exist

## Limitations

- Branch stages must be terminal (last stage in pipeline)
- Cannot branch to external pipelines (only inline pipelines)
- No support for multiple condition matching (only first match)
- No parallel execution of multiple branches

## Integration with Other Stages

The branch stage works well with:
- **Validation stages**: Ensure data structure before branching
- **Transform stages**: Prepare data for condition evaluation
- **Filter stages**: Can be used within branch pipelines for additional filtering

## Error Handling

- If a condition filter evaluation fails, the branch stage fails
- If an inline pipeline fails, the branch stage fails with the pipeline error
- No condition matching is considered success (not failure)