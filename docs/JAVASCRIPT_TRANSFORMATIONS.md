# JavaScript Transformations

The Event Router now supports JavaScript transformations using the `js_transform` pipeline stage. This allows you to write custom JavaScript code to transform, validate, and enrich your data.

## Features

- **Pure Go Implementation**: Uses the goja JavaScript engine (no external dependencies)
- **Security Sandbox**: Restricted execution environment with configurable security levels
- **Built-in Utilities**: Pre-loaded functions for common operations
- **Timeout Control**: Configurable execution timeouts to prevent infinite loops
- **Memory Limits**: Resource usage controls (configurable)
- **Script Caching**: Compiled scripts are cached for better performance

## Basic Usage

### Simple Transformation

```json
{
  "type": "js_transform",
  "config": {
    "script": "return {...input, processed: true, timestamp: now()};"
  }
}
```

### Function-Based Transformation

```json
{
  "type": "js_transform",
  "config": {
    "script": "function transform(input) { return input.name ? input.name.toUpperCase() : input; }"
  }
}
```

## Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `script` | string | **required** | JavaScript code to execute |
| `timeout_ms` | number | 5000 | Execution timeout in milliseconds |
| `memory_limit_mb` | number | 50 | Memory limit in MB |
| `enable_console` | boolean | false | Allow console.log calls |
| `sandbox.enabled` | boolean | true | Enable security sandbox |
| `globals` | object | {} | Global variables for the script |
| `error_handling` | string | "stop" | How to handle errors: "stop", "continue", "default" |

## Built-in Functions

### JSON Operations
- `jsonParse(string)` - Parse JSON string
- `jsonStringify(object)` - Convert object to JSON string

### Date/Time
- `now()` - Current timestamp in milliseconds
- `formatDate(timestamp, format)` - Format timestamp ("iso", "rfc", or custom format)

### Encoding
- `base64Encode(string)` - Encode string to base64
- `base64Decode(string)` - Decode base64 string
- `urlEncode(string)` - URL encode string
- `urlDecode(string)` - URL decode string

### Crypto
- `sha256(string)` - Generate SHA256 hash
- `md5(string)` - Generate MD5 hash

### Console (when enabled)
- `console.log(...)` - Log to output
- `console.error(...)` - Log errors

## Available Variables

Your JavaScript code has access to:

- `input` - The parsed input data (JSON object or string)
- `headers` - HTTP headers from the request
- `metadata` - Pipeline metadata
- `timestamp` - Request timestamp in milliseconds
- `id` - Request/pipeline execution ID

## Examples

### 1. Data Validation and Transformation

```json
{
  "type": "js_transform",
  "config": {
    "script": "function transform(input) { if (!input.email) { throw new Error('Email required'); } return { ...input, email: input.email.toLowerCase(), processed_at: formatDate(now(), 'iso') }; }",
    "error_handling": "stop"
  }
}
```

### 2. Complex Business Logic

```json
{
  "type": "js_transform",
  "config": {
    "script": "function transform(input) { const users = input.users || []; const processed = users.filter(u => u.active).map(u => ({ id: u.id, name: u.first_name + ' ' + u.last_name, hash: sha256(u.id.toString()) })); return { total: processed.length, users: processed, summary: { active_users: processed.length, processing_time: now() } }; }"
  }
}
```

### 3. Using Global Variables

```json
{
  "type": "js_transform",
  "config": {
    "script": "return { ...input, api_version: API_VERSION, environment: ENV };",
    "globals": {
      "API_VERSION": "v2.0",
      "ENV": "production"
    }
  }
}
```

### 4. Conditional Processing

```json
{
  "type": "js_transform",
  "config": {
    "script": "if (input.type === 'user') { return { id: input.id, name: input.name.toUpperCase(), type: 'USER_PROCESSED' }; } else if (input.type === 'order') { return { ...input, total: input.amount * 1.1, tax_applied: true }; } return input;"
  }
}
```

### 5. Array Processing

```json
{
  "type": "js_transform",
  "config": {
    "script": "function transform(input) { if (!input.items || !Array.isArray(input.items)) return input; const totalValue = input.items.reduce((sum, item) => sum + (item.price * item.quantity), 0); return { ...input, item_count: input.items.length, total_value: totalValue, average_item_value: totalValue / input.items.length }; }"
  }
}
```

## Security Features

### Sandbox Restrictions

When sandbox is enabled (default), the following restrictions apply:

- No access to `eval()` or `Function()` constructor
- No access to global object manipulation
- No access to Node.js APIs (require, process, etc.)
- Limited reflection capabilities
- Frozen object prototypes

### Configuration Example

```json
{
  "type": "js_transform",
  "config": {
    "script": "return {transformed: true};",
    "sandbox": {
      "enabled": true,
      "allow_reflection": false,
      "allow_eval": false
    },
    "timeout_ms": 3000,
    "memory_limit_mb": 25
  }
}
```

## Error Handling

### Error Handling Strategies

- **stop** (default): Stop pipeline execution on error
- **continue**: Continue to next stage, skip this transformation
- **default**: Use default/fallback behavior

### Example with Error Handling

```json
{
  "type": "js_transform",
  "config": {
    "script": "if (!input.required_field) { throw new Error('Missing required field'); } return input;",
    "error_handling": "continue"
  }
}
```

## Performance Tips

1. **Keep scripts simple** - Complex logic should be broken into multiple stages
2. **Use script caching** - Scripts are automatically compiled and cached
3. **Set appropriate timeouts** - Balance between functionality and performance
4. **Minimize memory usage** - Avoid creating large objects or arrays
5. **Use built-in functions** - They are optimized for performance

## Pipeline Integration

JavaScript transformations work seamlessly with other pipeline stages:

```json
{
  "id": "user-processing-pipeline",
  "name": "User Data Processing",
  "stages": [
    {
      "type": "validate",
      "config": {
        "schema": {"type": "object", "required": ["email"]}
      }
    },
    {
      "type": "js_transform",
      "config": {
        "script": "return {...input, email: input.email.toLowerCase(), processed: true};"
      }
    },
    {
      "type": "json_transform",
      "config": {
        "operation": "merge",
        "merge_data": {"version": "1.0"}
      }
    }
  ]
}
```

## Debugging

Enable console logging for debugging:

```json
{
  "type": "js_transform",
  "config": {
    "script": "console.log('Input:', input); const result = {...input, debug: true}; console.log('Output:', result); return result;",
    "enable_console": true
  }
}
```

The console output will appear in the Event Router logs.