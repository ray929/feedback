package redis

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var Client *redis.Client
var prefix string

func InitRedis() {
	host := getEnvOrDefault("REDIS_HOST", "127.0.0.1")
	port := getEnvOrDefault("REDIS_PORT", "6379")
	password := os.Getenv("REDIS_PASSWORD")
	db, _ := strconv.Atoi(getEnvOrDefault("REDIS_DB", "0"))
	prefix = getEnvOrDefault("REDIS_PREFIX", "feedback")

	Client = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", host, port),
		Password: password,
		DB:       db,
	})

	_, err := Client.Ping(context.Background()).Result()
	if err != nil {
		fmt.Printf("Warning: Redis connection failed (%v), running without Redis\n", err)
		Client = nil
	} else {
		fmt.Println("Redis connected successfully.")
	}
}

func Key(name string) string {
	if prefix != "" {
		return prefix + ":" + name
	}
	return name
}

func SaveSession(token string, ttl time.Duration) error {
	if Client == nil {
		return nil // No redis, skip
	}
	return Client.Set(context.Background(), Key("session:"+token), "1", ttl).Err()
}

func ValidateSession(token string) bool {
	if Client == nil {
		return token != "" // Fallback: accept any non-empty token
	}
	exists, err := Client.Exists(context.Background(), Key("session:"+token)).Result()
	return err == nil && exists > 0
}

func DeleteSession(token string) {
	if Client == nil {
		return
	}
	Client.Del(context.Background(), Key("session:"+token))
}

func SaveQuerySession(formID, token string, ttl time.Duration) error {
	if Client == nil {
		return nil
	}
	return Client.Set(context.Background(), Key("query:"+formID+":"+token), "1", ttl).Err()
}

func ValidateQuerySession(formID, token string) bool {
	if Client == nil {
		return token != ""
	}
	exists, err := Client.Exists(context.Background(), Key("query:"+formID+":"+token)).Result()
	return err == nil && exists > 0
}

// CheckRateLimit returns true if the action is allowed, false if rate limited.
// limit: max count, window: time window
func CheckRateLimit(ip string, limit int, window time.Duration) bool {
	if Client == nil {
		return true // No redis, skip rate limiting
	}
	key := Key("ratelimit:" + ip)
	count, err := Client.Incr(context.Background(), key).Result()
	if err != nil {
		return true // Fail open
	}
	if count == 1 {
		Client.Expire(context.Background(), key, window)
	}
	return count <= int64(limit)
}

func getEnvOrDefault(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}
