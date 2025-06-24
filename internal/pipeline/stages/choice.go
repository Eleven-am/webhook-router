package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"webhook-router/internal/pipeline/core"
	"webhook-router/internal/pipeline/expression"
)

func init() {
	Register("choice", &ChoiceExecutor{})
}

// ChoiceExecutor implements the choice stage
type ChoiceExecutor struct{}

// ChoiceAction represents the different choice patterns
type ChoiceAction struct {
	// For multi-case pattern
	CaseList []struct {
		When string          `json:"when"`
		Then json.RawMessage `json:"then"`
	} `json:"cases"`

	// For switch pattern
	Switch    string                     `json:"switch"`
	SwitchMap map[string]json.RawMessage `json:"cases,omitempty"`

	// Default value for both patterns
	Default json.RawMessage `json:"default"`
}

// Execute performs a choice operation
func (e *ChoiceExecutor) Execute(ctx context.Context, runCtx *core.Context, stage *core.StageDefinition) (interface{}, error) {
	// First try to parse as simple string (ternary)
	var simpleAction string
	if err := json.Unmarshal(stage.Action, &simpleAction); err == nil {
		// Handle ternary expression
		resolved, err := expression.ResolveTemplates(simpleAction, runCtx)
		if err != nil {
			return nil, fmt.Errorf("template resolution failed: %w", err)
		}

		result, err := expression.Evaluate(resolved, runCtx.GetAll())
		if err != nil {
			return nil, fmt.Errorf("ternary evaluation failed: %w", err)
		}

		return result, nil
	}

	// Parse as structured choice
	var choiceAction ChoiceAction
	if err := json.Unmarshal(stage.Action, &choiceAction); err != nil {
		return nil, fmt.Errorf("invalid choice action: %w", err)
	}

	// Handle multi-case pattern
	if len(choiceAction.CaseList) > 0 {
		return e.executeMultiCase(ctx, runCtx, choiceAction)
	}

	// Handle switch pattern
	if choiceAction.Switch != "" {
		return e.executeSwitch(ctx, runCtx, choiceAction)
	}

	return nil, fmt.Errorf("choice stage must have either cases or switch")
}

func (e *ChoiceExecutor) executeMultiCase(ctx context.Context, runCtx *core.Context, action ChoiceAction) (interface{}, error) {
	// Evaluate each case condition
	for _, c := range action.CaseList {
		// Resolve and evaluate condition
		condition, err := expression.ResolveTemplates(c.When, runCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve condition: %w", err)
		}

		result, err := expression.Evaluate(condition, runCtx.GetAll())
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate condition: %w", err)
		}

		// Check if condition is true
		if isTrue(result) {
			return e.parseThenValue(c.Then, runCtx)
		}
	}

	// No case matched, use default
	if action.Default != nil {
		return e.parseThenValue(action.Default, runCtx)
	}

	return nil, nil
}

func (e *ChoiceExecutor) executeSwitch(ctx context.Context, runCtx *core.Context, action ChoiceAction) (interface{}, error) {
	// Resolve switch expression
	switchValue, err := expression.ResolveTemplates(action.Switch, runCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve switch expression: %w", err)
	}

	// Evaluate to get the value
	value, err := expression.Evaluate(switchValue, runCtx.GetAll())
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate switch expression: %w", err)
	}

	// Convert to string for map lookup
	key := fmt.Sprint(value)

	// Look up in cases
	if caseValue, ok := action.SwitchMap[key]; ok {
		return e.parseThenValue(caseValue, runCtx)
	}

	// Use default
	if action.Default != nil {
		return e.parseThenValue(action.Default, runCtx)
	}

	return nil, nil
}

func (e *ChoiceExecutor) parseThenValue(raw json.RawMessage, runCtx *core.Context) (interface{}, error) {
	// Try to parse as string first
	var strValue string
	if err := json.Unmarshal(raw, &strValue); err == nil {
		// It's a string, might be an expression
		resolved, err := expression.ResolveTemplates(strValue, runCtx)
		if err != nil {
			return nil, err
		}

		// Check if it's an expression (contains operators or functions)
		if isExpression(resolved) {
			return expression.Evaluate(resolved, runCtx.GetAll())
		}

		return resolved, nil
	}

	// Parse as raw value
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("failed to parse then value: %w", err)
	}

	// Resolve templates if it's a complex structure
	return expression.ResolveTemplatesInValue(value, runCtx)
}

func isTrue(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case nil:
		return false
	case string:
		return v != ""
	case int, int64, float64:
		return true // Non-zero numbers are truthy
	default:
		return true // Non-nil values are truthy
	}
}

func isExpression(s string) bool {
	// Simple heuristic: contains operators or function calls
	operators := []string{"+", "-", "*", "/", "==", "!=", ">", "<", "&&", "||", "(", ")"}
	for _, op := range operators {
		if strings.Contains(s, op) {
			return true
		}
	}
	return false
}
