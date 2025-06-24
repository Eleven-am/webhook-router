package locks

import (
	"webhook-router/internal/redis"
)

// NewDistributedLockManager creates a distributed lock manager using the best available implementation.
//
// This function creates a RedsyncManager which uses the battle-tested Redlock algorithm
// implementation from go-redsync/redsync/v4. This provides better reliability and
// performance than the custom Redis locking implementation.
//
// The returned manager implements the same LockManagerInterface as the original
// Manager, making this a drop-in replacement.
//
// Parameters:
//   - redisClient: A connected Redis client instance for distributed coordination
//
// Returns:
//   - LockManagerInterface: A distributed lock manager (RedsyncManager)
//   - error: An error if the manager cannot be created
//
// Example:
//
//	redisClient, err := redis.NewClient(&redis.Config{
//		Address: "localhost:6379",
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	lockManager, err := locks.NewDistributedLockManager(redisClient)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer lockManager.Close()
func NewDistributedLockManager(redisClient *redis.Client) (LockManagerInterface, error) {
	return NewRedsyncManager(redisClient)
}
