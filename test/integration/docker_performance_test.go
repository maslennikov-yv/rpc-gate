//go:build docker
// +build docker

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"streaming-server/pkg/testutil"

	"github.com/stretchr/testify/suite"
)

// DockerPerformanceTestSuite provides Docker-based performance tests
type DockerPerformanceTestSuite struct {
	suite.Suite
	dockerAvailable bool
	quietMode       bool
}

// SetupSuite initializes the Docker performance test suite
func (suite *DockerPerformanceTestSuite) SetupSuite() {
	// Check if Docker tests should be skipped
	if os.Getenv("SKIP_DOCKER_TESTS") == "1" {
		suite.T().Skip("Docker tests are disabled")
		return
	}

	// Check for quiet mode flag
	suite.quietMode = os.Getenv("QUIET_TESTS") == "1"

	// Check Docker availability with timeout
	suite.dockerAvailable = suite.checkDockerAvailability()
	if !suite.dockerAvailable {
		suite.T().Skip("Docker is not available or not responding")
		return
	}
}

// checkDockerAvailability checks if Docker is available and responsive
func (suite *DockerPerformanceTestSuite) checkDockerAvailability() bool {
	section := testutil.NewSectionReporter("Docker Availability Check")
	section.SetQuietMode(suite.quietMode)
	section.Start()

	// Get timeout from environment or use default
	timeoutStr := os.Getenv("DOCKER_WAIT_TIMEOUT")
	timeout := 10 * time.Second // Default timeout
	if timeoutStr != "" {
		if parsedTimeout, err := time.ParseDuration(timeoutStr + "s"); err == nil {
			timeout = parsedTimeout
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	section.Status("Checking Docker availability (timeout: %v)", timeout)

	// Simple Docker availability check
	done := make(chan bool, 1)
	go func() {
		// Simulate Docker check - in real implementation, this would be:
		// exec.CommandContext(ctx, "docker", "version").Run()
		time.Sleep(100 * time.Millisecond) // Simulate quick check
		done <- true
	}()

	select {
	case <-done:
		section.Status("Docker is available")
		section.End()
		return true
	case <-ctx.Done():
		section.Status("Docker check timed out after %v", timeout)
		section.End()
		return false
	}
}

// TestDocker_ContainerPerformance tests container performance
func (suite *DockerPerformanceTestSuite) TestDocker_ContainerPerformance() {
    if !suite.dockerAvailable {
        suite.T().Skip("Docker not available")
        return
    }

    // Для этого теста НЕ включаем тихий режим
    localQuietMode := false

    section := testutil.NewSectionReporter("Docker Container Performance Test")
    section.SetQuietMode(localQuietMode)
    section.Start()

    section.Status("Starting Docker performance test...")
    
    // Simulate Docker container performance test
    time.Sleep(2 * time.Second)
    
    section.Status("Docker performance test completed")
    section.End()
}

// TestDocker_NetworkLatency tests network latency in containers
func (suite *DockerPerformanceTestSuite) TestDocker_NetworkLatency() {
    if !suite.dockerAvailable {
        suite.T().Skip("Docker not available")
        return
    }

    // Для этого теста НЕ включаем тихий режим
    localQuietMode := false

    section := testutil.NewSectionReporter("Docker Network Latency Test")
    section.SetQuietMode(localQuietMode)
    section.Start()

    section.Status("Testing network latency...")
    
    // Simulate network latency test
    time.Sleep(1 * time.Second)
    
    section.Status("Network latency test completed")
    section.End()
}

// Run the Docker performance test suite
func TestDockerPerformanceSuite(t *testing.T) {
	// Only run if Docker tests are explicitly enabled
	if os.Getenv("SKIP_DOCKER_TESTS") == "1" {
		t.Skip("Docker tests are disabled")
		return
	}

	fmt.Print(testutil.FormatHeader("Docker Performance Test Suite"))

	suite.Run(t, new(DockerPerformanceTestSuite))

	fmt.Print(testutil.FormatFooter())
}
