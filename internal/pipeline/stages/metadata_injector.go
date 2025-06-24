package stages

// MetadataInjector is an interface for stages that need pipeline metadata
type MetadataInjector interface {
	SetMetadata(pipelineID, userID string)
}

// InjectMetadata injects metadata into stages that support it
func InjectMetadata(registry *Registry, pipelineID, userID string) {
	// Inject metadata into cache stage
	if executor, found := registry.GetExecutor("cache"); found {
		if injector, ok := executor.(MetadataInjector); ok {
			injector.SetMetadata(pipelineID, userID)
		}
	}

	// Inject metadata into cache.invalidate stage
	if executor, found := registry.GetExecutor("cache.invalidate"); found {
		if injector, ok := executor.(MetadataInjector); ok {
			injector.SetMetadata(pipelineID, userID)
		}
	}
}
