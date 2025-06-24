package expression

// ContextResolver is an interface for resolving context values
type ContextResolver interface {
	GetPath(path string) (interface{}, bool)
}
