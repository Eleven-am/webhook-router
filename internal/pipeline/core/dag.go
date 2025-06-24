package core

import (
	"fmt"
	"sort"

	"github.com/heimdalr/dag"
)

// DAG represents the directed acyclic graph of pipeline stages
type DAG struct {
	graph     *dag.DAG
	stages    map[string]*StageDefinition
	vertexIDs map[string]string // Maps stage ID to vertex ID
	stageIDs  map[string]string // Maps vertex ID to stage ID
}

// NewDAG creates a new DAG from pipeline stages
func NewDAG(stages []StageDefinition) (*DAG, error) {
	d := &DAG{
		graph:     dag.NewDAG(),
		stages:    make(map[string]*StageDefinition),
		vertexIDs: make(map[string]string),
		stageIDs:  make(map[string]string),
	}

	// Add all stages as vertices first
	for i := range stages {
		stage := &stages[i]
		if _, exists := d.stages[stage.ID]; exists {
			return nil, fmt.Errorf("duplicate stage ID: %s", stage.ID)
		}

		// Add vertex to the graph - the vertex data is the stage ID itself
		vertexID, err := d.graph.AddVertex(stage.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to add stage '%s': %w", stage.ID, err)
		}

		d.stages[stage.ID] = stage
		d.vertexIDs[stage.ID] = vertexID
		d.stageIDs[vertexID] = stage.ID
	}

	// Add edges based on dependencies
	for _, stage := range stages {
		for _, dep := range stage.DependsOn {
			if _, exists := d.stages[dep]; !exists {
				return nil, fmt.Errorf("stage '%s' depends on non-existent stage '%s'", stage.ID, dep)
			}

			// Add edge from dependency to stage (dep -> stage)
			// In heimdalr/dag, edge direction is from -> to
			// Since stage depends on dep, dep must complete before stage
			// So edge goes from dep to stage
			depVertexID := d.vertexIDs[dep]
			stageVertexID := d.vertexIDs[stage.ID]
			err := d.graph.AddEdge(depVertexID, stageVertexID)
			if err != nil {
				// This will fail if it creates a cycle
				return nil, fmt.Errorf("adding edge from '%s' to '%s' failed: %w", dep, stage.ID, err)
			}
		}
	}

	return d, nil
}

// GetExecutionPlan returns batches of stages that can run in parallel
func (d *DAG) GetExecutionPlan() ([][]string, error) {
	var plan [][]string
	completed := make(map[string]bool)
	remaining := make(map[string]bool)

	// Initialize remaining stages
	for id := range d.stages {
		remaining[id] = true
	}

	// Build execution batches using the graph's built-in methods
	for len(remaining) > 0 {
		batch := []string{}

		// Find all stages that can run now (no incomplete dependencies)
		for id := range remaining {
			canRun := true

			// Get ancestors (dependencies) of this stage
			vertexID := d.vertexIDs[id]
			ancestors, err := d.graph.GetAncestors(vertexID)
			if err != nil {
				return nil, fmt.Errorf("failed to get dependencies for stage '%s': %w", id, err)
			}

			// Check if all dependencies are completed
			for depVertexID := range ancestors {
				depStageID := d.stageIDs[depVertexID]
				if !completed[depStageID] {
					canRun = false
					break
				}
			}

			if canRun {
				batch = append(batch, id)
			}
		}

		if len(batch) == 0 {
			// This shouldn't happen if DAG validation passed
			return nil, fmt.Errorf("deadlock detected in execution plan")
		}

		// Sort batch for deterministic execution order
		sort.Strings(batch)

		// Mark batch as completed and remove from remaining
		for _, id := range batch {
			completed[id] = true
			delete(remaining, id)
		}

		plan = append(plan, batch)
	}

	return plan, nil
}

// The heimdalr/dag library automatically checks for cycles when adding edges,
// so we don't need a separate validateNoCycles method

// GetStage returns a stage definition by ID
func (d *DAG) GetStage(id string) (*StageDefinition, bool) {
	stage, ok := d.stages[id]
	return stage, ok
}

// GetDependents returns all stages that depend on the given stage
func (d *DAG) GetDependents(stageID string) []string {
	// Get vertex ID for this stage
	vertexID, exists := d.vertexIDs[stageID]
	if !exists {
		return []string{}
	}

	// Get descendants (stages that depend on this stage)
	descendants, err := d.graph.GetDescendants(vertexID)
	if err != nil {
		return []string{}
	}

	// Convert to stage IDs
	var dependents []string
	for descVertexID := range descendants {
		if descStageID, ok := d.stageIDs[descVertexID]; ok {
			dependents = append(dependents, descStageID)
		}
	}

	return dependents
}

// GetDependencies returns all stages that the given stage depends on
func (d *DAG) GetDependencies(stageID string) []string {
	// Get vertex ID for this stage
	vertexID, exists := d.vertexIDs[stageID]
	if !exists {
		return []string{}
	}

	// Get ancestors (stages this stage depends on)
	ancestors, err := d.graph.GetAncestors(vertexID)
	if err != nil {
		return []string{}
	}

	// Convert to stage IDs
	var dependencies []string
	for ancVertexID := range ancestors {
		if ancStageID, ok := d.stageIDs[ancVertexID]; ok {
			dependencies = append(dependencies, ancStageID)
		}
	}

	return dependencies
}
