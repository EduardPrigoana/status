package main

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port                string
	CheckInterval       time.Duration
	InstancesURL        string
	RequestTimeout      time.Duration
	MaxCheckHistory     int
	SSEKeepaliveSeconds int
	LogLevel            string
}

func LoadConfig() *Config {
	config := &Config{
		Port:                getEnv("PORT", "8080"),
		CheckInterval:       getCheckInterval(),
		InstancesURL:        getEnv("INSTANCES_URL", "https://raw.githubusercontent.com/EduardPrigoana/hifi-instances/refs/heads/main/instances.json"),
		RequestTimeout:      getTimeout(),
		MaxCheckHistory:     getMaxHistory(),
		SSEKeepaliveSeconds: getSSEKeepalive(),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
	}

	if !strings.HasPrefix(config.Port, ":") {
		config.Port = ":" + config.Port
	}

	return config
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getCheckInterval() time.Duration {
	intervalStr := os.Getenv("CHECK_INTERVAL_MINUTES")
	if intervalStr == "" {
		return 60 * time.Minute
	}

	minutes, err := strconv.Atoi(intervalStr)
	if err != nil {
		log.Printf("Invalid CHECK_INTERVAL_MINUTES value '%s', using default 60 minutes", intervalStr)
		return 60 * time.Minute
	}

	if minutes < 1 {
		log.Printf("CHECK_INTERVAL_MINUTES must be at least 1, using default 60 minutes")
		return 60 * time.Minute
	}

	return time.Duration(minutes) * time.Minute
}

func getTimeout() time.Duration {
	timeoutStr := os.Getenv("REQUEST_TIMEOUT_SECONDS")
	if timeoutStr == "" {
		return 30 * time.Second
	}

	seconds, err := strconv.Atoi(timeoutStr)
	if err != nil || seconds < 1 {
		log.Printf("Invalid REQUEST_TIMEOUT_SECONDS, using default 30 seconds")
		return 30 * time.Second
	}

	return time.Duration(seconds) * time.Second
}

func getMaxHistory() int {
	historyStr := os.Getenv("MAX_CHECK_HISTORY")
	if historyStr == "" {
		return 168
	}

	history, err := strconv.Atoi(historyStr)
	if err != nil || history < 1 {
		log.Printf("Invalid MAX_CHECK_HISTORY, using default 168")
		return 168
	}

	return history
}

func getSSEKeepalive() int {
	keepaliveStr := os.Getenv("SSE_KEEPALIVE_SECONDS")
	if keepaliveStr == "" {
		return 30
	}

	seconds, err := strconv.Atoi(keepaliveStr)
	if err != nil || seconds < 1 {
		log.Printf("Invalid SSE_KEEPALIVE_SECONDS, using default 30")
		return 30
	}

	return seconds
}

func (c *Config) LogConfig() {
	log.Printf("Configuration:")
	log.Printf("  Port: %s", c.Port)
	log.Printf("  Check Interval: %v", c.CheckInterval)
	log.Printf("  Instances URL: %s", c.InstancesURL)
	log.Printf("  Request Timeout: %v", c.RequestTimeout)
	log.Printf("  Max Check History: %d", c.MaxCheckHistory)
	log.Printf("  SSE Keepalive: %ds", c.SSEKeepaliveSeconds)
	log.Printf("  Log Level: %s", c.LogLevel)
}
