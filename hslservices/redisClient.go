package fleetbridge

import (
	"context"
	"fmt"
	"os"
	"strconv"

	redis "github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

var (
	redisHost = os.Getenv("REDIS_HOST")
	redisPort = os.Getenv("REDIS_PORT")
	redisDB   = os.Getenv("REDIS_DB")
)

// onConnectRedisHandler - Light Wrapper implements redis.OnConnect Func.
func onConnectRedisHandler(ctx context.Context, cn *redis.Conn) error {
	log.Info("New Redis Client Connection Established")
	return nil
}

// InitRedisClient - Define the Handlers to Execute on Connect, Message, and Disconnect
func InitRedisClient(ctx context.Context) *redis.Client {

	redisDBID, err := strconv.Atoi(redisDB)

	if err != nil {
		log.WithFields(log.Fields{
			"RedisDB": redisDB,
		}).Errorf("Invalid Redis DB (%s): %s", redisDB, err)
	}

	client := redis.NewClient(&redis.Options{
		Addr:       fmt.Sprintf("%s:%s", redisHost, redisPort),
		Password:   os.Getenv("REDISCLI_AUTH"),
		DB:         redisDBID,
		MaxRetries: 5,
		OnConnect:  onConnectRedisHandler,
	})

	// Confirm connection
	_, err = client.Ping(ctx).Result()
	if err != nil {
		log.WithFields(log.Fields{
			"Addr": fmt.Sprintf("%s:%s", redisHost, redisPort),
		}).Errorf("Invalid Redis DB (%s): %s", redisDB, err)
		log.Panicln(err)
	}

	return client
}
