package integration

import (
	"os"
	"strconv"
	"time"
)

// TestConfig holds configuration for integration tests
type TestConfig struct {
	HTTPPort     int
	HTTPSPort    int
	TCPPort      int
	TLSPort      int
	WSPort       int
	WSSPort      int
	TestTimeout  time.Duration
	SlowTimeout  time.Duration
	LoadTestSize int
}

// GetTestConfig returns test configuration from environment or defaults
func GetTestConfig() TestConfig {
	config := TestConfig{
		HTTPPort:     18080,
		HTTPSPort:    18443,
		TCPPort:      18081,
		TLSPort:      18444,
		WSPort:       18082,
		WSSPort:      18445,
		TestTimeout:  30 * time.Second,
		SlowTimeout:  60 * time.Second,
		LoadTestSize: 100,
	}

	if port := os.Getenv("TEST_HTTP_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.HTTPPort = p
		}
	}

	if port := os.Getenv("TEST_WS_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.WSPort = p
		}
	}

	if timeout := os.Getenv("TEST_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err == nil {
			config.TestTimeout = t
		}
	}

	if size := os.Getenv("LOAD_TEST_SIZE"); size != "" {
		if s, err := strconv.Atoi(size); err == nil {
			config.LoadTestSize = s
		}
	}

	return config
}
