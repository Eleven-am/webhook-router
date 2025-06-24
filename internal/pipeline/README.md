# Pipeline Execution Engine

This is the core pipeline execution engine that implements the DAG-based pipeline architecture specified in `/NEW_PIPELINE_ARCHITECTURE_SPEC.md`.

## Structure

```
/pipeline
├── core/           # Core pipeline types and execution engine
├── stages/         # Stage type implementations
├── expression/     # Template and expression handling
├── errors/         # Custom error types
└── testutil/       # Testing utilities
```

## Key Components

- **Core**: Pipeline definitions, DAG execution, and context management
- **Stages**: Individual stage executors (transform, http, choice, etc.)
- **Expression**: Template resolution and expr-lang integration
- **Errors**: Structured error types for better debugging

## Usage

```go
import (
    "github.com/yourproject/pipeline/core"
    "github.com/yourproject/pipeline/stages"
)

// Load pipeline from JSON
p, err := core.LoadPipeline(jsonData)

// Create executor
executor := core.NewExecutor()

// Execute with input
result, err := executor.Execute(ctx, p, inputData)
```