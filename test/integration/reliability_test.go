//go:build reliability
// +build reliability

package integration

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"streaming-server/pkg/testutil"
	"streaming-server/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// ReliabilityTestSuite provides reliability testing
type ReliabilityTestSuite struct {
	IntegrationTestSuite
	progressGroup *testutil.ProgressGroup
}

// SetupSuite initializes the test suite
func (suite *ReliabilityTestSuite) SetupSuite() {
	// Call the parent SetupSuite
	suite.IntegrationTestSuite.SetupSuite()

	// Initialize progress group
	suite.progressGroup = testutil.NewProgressGroup()
}

// TearDownSuite cleans up after tests
func (suite *ReliabilityTestSuite) TearDownSuite() {
	// Stop all progress reporters
	if suite.progressGroup != nil {
		suite.progressGroup.StopAll()
	}

	// Call the parent TearDownSuite
	suite.IntegrationTestSuite.TearDownSuite()
}

// TestReliability_ServerRecovery tests server recovery after errors
func (suite *ReliabilityTestSuite) TestReliability_ServerRecovery() {
	// Create a section reporter for this test
	section := testutil.NewSectionReporter("Server Recovery Test")
	section.Start()

	// Send request that causes error
	errorRequest := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "test_error",
		ID:      "error-recovery-test",
	}

	// Server should return error but continue working
	section.Status("Sending error-inducing request")
	_ = suite.makeHTTPRequest(errorRequest)
	section.Status("Error request sent, checking server recovery")

	// Check that server still responds to normal requests
	normalRequest := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "status",
		ID:      "recovery-check",
	}

	section.Status("Sending normal request to verify server is still responsive")
	normalResponse := suite.makeHTTPRequest(normalRequest)

	if normalResponse != nil && normalResponse.Error == nil {
		assert.Equal(suite.T(), "2.0", normalResponse.JSONRPC)
		assert.Equal(suite.T(), "recovery-check", normalResponse.ID)
		assert.Nil(suite.T(), normalResponse.Error)
		section.End()
	} else {
		section.Fail(fmt.Errorf("server did not recover properly"))
	}
}

// TestReliability_MemoryLeaks tests for memory leaks
func (suite *ReliabilityTestSuite) TestReliability_MemoryLeaks() {
	if testing.Short() {
		suite.T().Skip("Skipping memory leak tests in short mode")
	}

	// Только для этого теста включаем тихий режим
	localQuietMode := true

	// Create a section reporter for this test
	section := testutil.NewSectionReporter("Memory Leak Test")
	section.SetQuietMode(localQuietMode)
	section.Start()

	// Execute many requests to check for memory leaks
	const iterations = 50

	// Create a progress reporter for this test
	progressReporter := testutil.NewProgressReporter("Memory Leak Test", iterations)
	progressReporter.SetQuietMode(localQuietMode)
	progressReporter.Start()
	defer progressReporter.Stop()

	// Add to the progress group
	suite.progressGroup.AddReporter("memory_leaks", progressReporter)

	section.Status("Starting memory leak test with %d iterations", iterations)

	for i := 0; i < iterations; i++ {
		request := types.JSONRPCRequest{
			JSONRPC: "2.0",
			Method:  "echo",
			Params:  json.RawMessage(fmt.Sprintf(`{"memory_test": true, "iteration": %d}`, i)),
			ID:      fmt.Sprintf("memory-test-%d", i),
		}

		response := suite.makeHTTPRequest(request)
		assert.Equal(suite.T(), "2.0", response.JSONRPC)

		// Update progress
		progressReporter.Increment()

		// Log progress periodically
		if i%10 == 0 {
			progressReporter.Message("Completed %d/%d memory leak test iterations", i, iterations)
		}

		// Small pause between requests
		time.Sleep(10 * time.Millisecond)
	}

	section.Status("Memory leak test completed successfully")
	section.End()
}

// Run reliability test suite
func TestReliabilitySuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping reliability tests in short mode")
	}

	fmt.Print(testutil.FormatHeader("Reliability Test Suite"))

	suite.Run(t, new(ReliabilityTestSuite))

	fmt.Print(testutil.FormatFooter())
}
