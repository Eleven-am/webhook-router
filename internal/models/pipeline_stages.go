package models

// PipelineConfig represents the main pipeline configuration
// @Description Pipeline configuration for data transformation
type PipelineConfig struct {
	ID             string                 `json:"id" example:"pipe_123"`
	Name           string                 `json:"name" example:"Data Processing Pipeline"`
	Version        string                 `json:"version" example:"1.0.0"`
	DefaultTimeout string                 `json:"defaultTimeout" example:"30s"`
	Stages         []PipelineStageConfig  `json:"stages"`
	Resources      map[string]interface{} `json:"resources,omitempty"`
}

// PipelineStageConfig represents a single stage in the pipeline
// @Description Individual stage configuration within a pipeline
type PipelineStageConfig struct {
	ID        string      `json:"id" example:"transform_1"`
	Type      string      `json:"type" example:"transform" enums:"input,output,transform,http,validate,filter,branch,choice,cache,publish,foreach,metadata"`
	Target    interface{} `json:"target,omitempty" swaggertype:"string" example:"data.result"`
	Action    interface{} `json:"action,omitempty" swaggertype:"object"`
	DependsOn []string    `json:"dependsOn,omitempty" example:"[\"input\", \"validate\"]"`
	OnError   interface{} `json:"onError,omitempty" swaggertype:"object"`
	Timeout   string      `json:"timeout,omitempty" example:"30s"`
	Items     interface{} `json:"items,omitempty" swaggertype:"string" example:"data.items"`
	Filter    interface{} `json:"filter,omitempty" swaggertype:"object"`
}
