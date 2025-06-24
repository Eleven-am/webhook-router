package app

import (
	"context"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/pipeline"
	"webhook-router/internal/routing"
)

func (app *App) initializeRouting() {
	ruleEngine := routing.NewBasicRuleEngine()
	destManager := routing.NewBasicDestinationManager()
	app.Router = routing.NewBasicRouter(ruleEngine, destManager)
	app.Logger.Info("Routing Engine: Started")
}

func (app *App) initializePipeline(ctx context.Context) {
	// Create new pipeline engine
	engine := pipeline.NewEngine()

	// Start the engine (registers all stage types)
	if err := engine.Start(ctx); err != nil {
		app.Logger.Warn("Failed to start pipeline engine", logging.Field{"error", err})
		return
	}

	app.PipelineEngine = engine
	app.Logger.Info("Pipeline Engine: Started successfully")
}
