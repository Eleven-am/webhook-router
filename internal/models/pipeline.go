package models

import (
	"time"
)

// PipelineAPI represents a pipeline configuration for API responses
type PipelineAPI struct {
	ID             string            `json:"id" example:"pipe_123"`
	Name           string            `json:"name" example:"Data Processing Pipeline"`
	Version        string            `json:"version,omitempty" example:"1.0.0"`
	Description    string            `json:"description,omitempty" example:"Processes incoming webhook data"`
	Stages         []StageAPI        `json:"stages"`
	DefaultTimeout string            `json:"default_timeout,omitempty" example:"30s"`
	Resources      map[string]string `json:"resources,omitempty"`
	CreatedAt      time.Time         `json:"created_at,omitempty"`
	UpdatedAt      time.Time         `json:"updated_at,omitempty"`
}

// StageAPI represents a pipeline stage for API responses
type StageAPI struct {
	ID        string         `json:"id" example:"transform_1"`
	Type      string         `json:"type" example:"transform" enums:"transform,http,validate,filter,branch,cache,publish,foreach,parallel"`
	Target    interface{}    `json:"target" swaggertype:"string" example:"data.user"`
	DependsOn []string       `json:"depends_on,omitempty" example:"['validate_1']"`
	Timeout   string         `json:"timeout,omitempty" example:"10s"`
	OnError   *ErrorHandling `json:"on_error,omitempty"`
	Cache     *CacheConfig   `json:"cache,omitempty"`

	// Stage-specific configuration (only one should be set based on type)
	Transform *TransformAction `json:"transform,omitempty"`
	HTTP      *HTTPAction      `json:"http,omitempty"`
	Validate  *ValidateAction  `json:"validate,omitempty"`
	Filter    *FilterAction    `json:"filter,omitempty"`
	Branch    *BranchAction    `json:"branch,omitempty"`
	Publish   *PublishAction   `json:"publish,omitempty"`
	Foreach   *ForeachAction   `json:"foreach,omitempty"`
	Parallel  *ParallelAction  `json:"parallel,omitempty"`
}

// ErrorHandling defines how to handle stage errors
type ErrorHandling struct {
	Strategy string      `json:"strategy" example:"continue" enums:"continue,retry,fail,fallback"`
	Retries  int         `json:"retries,omitempty" example:"3"`
	Delay    string      `json:"delay,omitempty" example:"1s"`
	Fallback interface{} `json:"fallback,omitempty" swaggertype:"string" example:"default_value"`
}

// CacheConfig defines caching behavior for a stage
type CacheConfig struct {
	Key string `json:"key" example:"user_${data.user_id}"`
	TTL string `json:"ttl" example:"5m"`
}

// TransformAction defines a data transformation
type TransformAction struct {
	Type       string                 `json:"type" example:"set" enums:"set,delete,copy,move,merge,template,jq,javascript"`
	Value      interface{}            `json:"value,omitempty" swaggertype:"string" example:"processed"`
	From       string                 `json:"from,omitempty" example:"data.source"`
	To         string                 `json:"to,omitempty" example:"data.destination"`
	Template   string                 `json:"template,omitempty" example:"Hello ${data.name}"`
	Expression string                 `json:"expression,omitempty" example:".data | map(select(.active))"`
	Script     string                 `json:"script,omitempty" example:"return data.map(d => d.value * 2);"`
	Fields     map[string]interface{} `json:"fields,omitempty" swaggertype:"object"`
}

// HTTPAction defines an HTTP request stage
type HTTPAction struct {
	Method      string            `json:"method" example:"POST" enums:"GET,POST,PUT,PATCH,DELETE"`
	URL         string            `json:"url" example:"https://api.example.com/webhook"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        interface{}       `json:"body,omitempty" swaggertype:"object"`
	QueryParams map[string]string `json:"query_params,omitempty"`
	Timeout     string            `json:"timeout,omitempty" example:"30s"`
	Retry       *RetryConfig      `json:"retry,omitempty"`
	Auth        *AuthConfig       `json:"auth,omitempty"`
}

// ValidateAction defines validation rules
type ValidateAction struct {
	Type       string                 `json:"type" example:"schema" enums:"required,type,range,regex,custom,schema"`
	Required   []string               `json:"required,omitempty" example:"['user_id','email']"`
	Properties map[string]interface{} `json:"properties,omitempty" swaggertype:"object"`
	Pattern    string                 `json:"pattern,omitempty" example:"^[A-Z]{3}[0-9]{4}$"`
	Min        interface{}            `json:"min,omitempty" swaggertype:"number" example:"0"`
	Max        interface{}            `json:"max,omitempty" swaggertype:"number" example:"100"`
	Schema     interface{}            `json:"schema,omitempty" swaggertype:"object"`
	Message    string                 `json:"message,omitempty" example:"Invalid format"`
}

// FilterAction defines filtering criteria
type FilterAction struct {
	Type       string `json:"type" example:"expression" enums:"expression,javascript,jq"`
	Expression string `json:"expression" example:"data.status == 'active'"`
	Script     string `json:"script,omitempty" example:"return data.filter(d => d.active);"`
	JQ         string `json:"jq,omitempty" example:".[] | select(.status == \"active\")"`
	Keep       bool   `json:"keep,omitempty" example:"true"`
	Target     string `json:"target,omitempty" example:"items"`
}

// BranchAction defines conditional branching
type BranchAction struct {
	Conditions []BranchCondition `json:"conditions"`
	Default    string            `json:"default,omitempty" example:"fallback_stage"`
}

// BranchCondition defines a single branch condition
type BranchCondition struct {
	Expression string `json:"expression" example:"data.type == 'order'"`
	Target     string `json:"target" example:"process_order"`
}

// PublishAction defines message publishing
type PublishAction struct {
	Broker  string            `json:"broker" example:"main_broker"`
	Topic   string            `json:"topic" example:"processed_events"`
	Key     string            `json:"key,omitempty" example:"${data.user_id}"`
	Headers map[string]string `json:"headers,omitempty"`
}

// ForeachAction defines iteration over items
type ForeachAction struct {
	Items    string   `json:"items" example:"data.orders"`
	Variable string   `json:"variable,omitempty" example:"order"`
	Index    string   `json:"index,omitempty" example:"idx"`
	Stages   []string `json:"stages" example:"['process_item','save_item']"`
	Parallel bool     `json:"parallel,omitempty" example:"true"`
	MaxItems int      `json:"max_items,omitempty" example:"100"`
}

// ParallelAction defines parallel stage execution
type ParallelAction struct {
	Stages       []string `json:"stages" example:"['fetch_user','fetch_orders','fetch_profile']"`
	WaitAll      bool     `json:"wait_all,omitempty" example:"true"`
	MergeResults bool     `json:"merge_results,omitempty" example:"true"`
	TargetField  string   `json:"target_field,omitempty" example:"results"`
}

// RetryConfig defines retry behavior
type RetryConfig struct {
	MaxAttempts int    `json:"max_attempts" example:"3"`
	Delay       string `json:"delay" example:"1s"`
	MaxDelay    string `json:"max_delay,omitempty" example:"30s"`
	Backoff     string `json:"backoff,omitempty" example:"exponential" enums:"constant,linear,exponential"`
}

// AuthConfig defines authentication for HTTP requests
type AuthConfig struct {
	Type     string        `json:"type" example:"bearer" enums:"none,basic,bearer,api_key,oauth2"`
	Username string        `json:"username,omitempty" example:"user123"`
	Password string        `json:"password,omitempty" example:"pass123"`
	Token    string        `json:"token,omitempty" example:"${env.API_TOKEN}"`
	Key      string        `json:"key,omitempty" example:"X-API-Key"`
	Value    string        `json:"value,omitempty" example:"${secrets.api_key}"`
	OAuth2   *OAuth2Config `json:"oauth2,omitempty"`
}

// OAuth2Config defines OAuth2 authentication
type OAuth2Config struct {
	ServiceID string   `json:"service_id" example:"google_calendar"`
	Scopes    []string `json:"scopes,omitempty" example:"['read','write']"`
}

// PipelineExecutionRequest represents a request to execute a pipeline
type PipelineExecutionRequest struct {
	PipelineID string                 `json:"pipeline_id,omitempty" example:"pipe_123"`
	Pipeline   *PipelineAPI           `json:"pipeline,omitempty"`
	Data       interface{}            `json:"data" swaggertype:"object" example:"{\"user_id\":\"123\",\"action\":\"create\"}"`
	Context    map[string]interface{} `json:"context,omitempty" swaggertype:"object"`
	Timeout    string                 `json:"timeout,omitempty" example:"60s"`
}

// PipelineExecutionResponse represents the result of pipeline execution
type PipelineExecutionResponse struct {
	Success       bool                   `json:"success" example:"true"`
	Result        interface{}            `json:"result,omitempty" swaggertype:"object"`
	Error         string                 `json:"error,omitempty"`
	ExecutionTime string                 `json:"execution_time" example:"125ms"`
	StageResults  []StageExecutionResult `json:"stage_results,omitempty"`
}

// StageExecutionResult represents the result of a single stage execution
type StageExecutionResult struct {
	StageID  string      `json:"stage_id" example:"transform_1"`
	Success  bool        `json:"success" example:"true"`
	Duration string      `json:"duration" example:"25ms"`
	Input    interface{} `json:"input,omitempty" swaggertype:"object"`
	Output   interface{} `json:"output,omitempty" swaggertype:"object"`
	Error    string      `json:"error,omitempty"`
	Cached   bool        `json:"cached,omitempty" example:"false"`
}
