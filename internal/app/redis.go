package app

import (
	"strconv"

	"webhook-router/internal/common/logging"
	"webhook-router/internal/redis"
)

func (app *App) initializeRedis() error {
	if app.Config.RedisAddress == "" {
		app.Logger.Info("Redis: Not configured (rate limiting and distributed locks disabled)")
		return nil
	}

	// Convert config values
	redisDB, _ := strconv.Atoi(app.Config.RedisDB)
	redisPoolSize, _ := strconv.Atoi(app.Config.RedisPoolSize)

	redisConfig := &redis.Config{
		Address:  app.Config.RedisAddress,
		Password: app.Config.RedisPassword,
		DB:       redisDB,
		PoolSize: redisPoolSize,
	}

	redisClient, err := redis.NewClient(redisConfig)
	if err != nil {
		return err
	}

	app.RedisClient = redisClient
	app.Logger.Info("Redis: Connected", logging.Field{"address", app.Config.RedisAddress})

	// Log rate limiting configuration
	defaultLimit, _ := strconv.Atoi(app.Config.RateLimitDefault)
	if defaultLimit == 0 {
		defaultLimit = 100
	}
	window := app.Config.RateLimitWindow
	if window == "" {
		window = "1m"
	}
	app.Logger.Info("Rate Limiting: Enabled",
		logging.Field{"limit", defaultLimit},
		logging.Field{"window", window},
	)
	app.Logger.Info("Distributed Locks: Enabled")

	return nil
}
