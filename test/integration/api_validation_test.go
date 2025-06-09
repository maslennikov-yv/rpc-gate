package integration

import (
	"encoding/json"
	"fmt"

	"streaming-server/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAPI_EchoValidation tests the echo API with various input parameters
func (suite *IntegrationTestSuite) TestAPI_EchoValidation() {
	testCases := []struct {
		name          string
		params        string
		expectedError bool
		validate      func(t assert.TestingT, result map[string]interface{})
	}{
		{
			name:          "Valid Echo",
			params:        `{"message": "test message", "timestamp": "2023-01-01T00:00:00Z"}`,
			expectedError: false,
			validate: func(t assert.TestingT, result map[string]interface{}) {
				echo, ok := result["echo"].(map[string]interface{})
				require.True(suite.T(), ok)
				assert.Equal(t, "test message", echo["message"])
				assert.Equal(t, "2023-01-01T00:00:00Z", echo["timestamp"])
			},
		},
		{
			name:          "Missing Message",
			params:        `{"timestamp": "2023-01-01T00:00:00Z"}`,
			expectedError: false,
			validate: func(t assert.TestingT, result map[string]interface{}) {
				echo, ok := result["echo"].(map[string]interface{})
				require.True(suite.T(), ok)
				// The server may or may not include empty fields in the response
				// We just verify that the timestamp is echoed correctly
				assert.Equal(t, "2023-01-01T00:00:00Z", echo["timestamp"])
			},
		},
		{
			name:          "Invalid Timestamp",
			params:        `{"message": "test", "timestamp": "invalid-time"}`,
			expectedError: false,
			validate: func(t assert.TestingT, result map[string]interface{}) {
				echo, ok := result["echo"].(map[string]interface{})
				require.True(suite.T(), ok)
				assert.Equal(t, "test", echo["message"])
				assert.Equal(t, "invalid-time", echo["timestamp"])
			},
		},
		{
			name:          "Additional Fields",
			params:        `{"message": "test", "timestamp": "2023-01-01T00:00:00Z", "extra": "field"}`,
			expectedError: false,
			validate: func(t assert.TestingT, result map[string]interface{}) {
				echo, ok := result["echo"].(map[string]interface{})
				require.True(suite.T(), ok)
				assert.Equal(t, "test", echo["message"])
				assert.Equal(t, "2023-01-01T00:00:00Z", echo["timestamp"])
				_, hasExtra := echo["extra"]
				assert.True(t, hasExtra, "Extra field should be included in echo response")
			},
		},
	}

	for i, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %d: %s", i, tc.name), func() {
			request := types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "echo",
				Params:  json.RawMessage(tc.params),
				ID:      fmt.Sprintf("echo-test-%d", i),
			}

			response := suite.makeHTTPRequest(request)

			if tc.expectedError {
				assert.NotNil(suite.T(), response.Error)
			} else {
				assert.Nil(suite.T(), response.Error)
				assert.NotNil(suite.T(), response.Result)

				result, ok := response.Result.(map[string]interface{})
				require.True(suite.T(), ok)
				tc.validate(suite.T(), result)
			}
		})
	}
}

// TestAPI_CalculateValidation tests the calculate API with various input parameters
func (suite *IntegrationTestSuite) TestAPI_CalculateValidation() {
	testCases := []struct {
		name          string
		params        string
		expectedError bool
		errorCode     int
		errorMessage  string
		expectedValue interface{}
	}{
		{
			name:          "Valid Addition",
			params:        `{"operation": "add", "a": 5, "b": 3}`,
			expectedError: false,
			expectedValue: float64(8),
		},
		{
			name:          "Valid Subtraction",
			params:        `{"operation": "subtract", "a": 5, "b": 3}`,
			expectedError: false,
			expectedValue: float64(2),
		},
		{
			name:          "Valid Multiplication",
			params:        `{"operation": "multiply", "a": 5, "b": 3}`,
			expectedError: false,
			expectedValue: float64(15),
		},
		{
			name:          "Valid Division",
			params:        `{"operation": "divide", "a": 6, "b": 3}`,
			expectedError: false,
			expectedValue: float64(2),
		},
		{
			name:          "Division by Zero",
			params:        `{"operation": "divide", "a": 5, "b": 0}`,
			expectedError: true,
			errorCode:     -32602,
			errorMessage:  "Invalid params: Division by zero",
		},
		{
			name:          "Invalid Operation",
			params:        `{"operation": "invalid", "a": 5, "b": 3}`,
			expectedError: true,
			errorCode:     -32602,
			errorMessage:  "Invalid params: Invalid operation",
		},
		{
			name:          "Missing Operation",
			params:        `{"a": 5, "b": 3}`,
			expectedError: true,
			errorCode:     -32602,
			errorMessage:  "Invalid params: Missing required parameter",
		},
		{
			name:          "Missing Operands",
			params:        `{"operation": "add"}`,
			expectedError: true,
			errorCode:     -32602,
			errorMessage:  "Invalid params: Missing required parameters",
		},
		{
			name:          "String Operands",
			params:        `{"operation": "add", "a": "5", "b": "3"}`,
			expectedError: true,
			errorCode:     -32602,
			errorMessage:  "Invalid params: Failed to parse parameters",
		},
	}

	for i, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %d: %s", i, tc.name), func() {
			request := types.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "calculate",
				Params:  json.RawMessage(tc.params),
				ID:      fmt.Sprintf("calc-test-%d", i),
			}

			response := suite.makeHTTPRequest(request)

			if tc.expectedError {
				assert.NotNil(suite.T(), response.Error)
				if response.Error != nil {
					assert.Equal(suite.T(), tc.errorCode, response.Error.Code)
					assert.Contains(suite.T(), response.Error.Message, tc.errorMessage)
				}
			} else {
				assert.Nil(suite.T(), response.Error)
				assert.NotNil(suite.T(), response.Result)

				result, ok := response.Result.(map[string]interface{})
				require.True(suite.T(), ok)
				assert.Equal(suite.T(), tc.expectedValue, result["result"])
			}
		})
	}
}

// TestAPI_TimeValidation tests the time API
func (suite *IntegrationTestSuite) TestAPI_TimeValidation() {
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "time",
		ID:      "time-test",
	}

	response := suite.makeHTTPRequest(request)

	assert.Nil(suite.T(), response.Error)
	assert.NotNil(suite.T(), response.Result)

	result, ok := response.Result.(map[string]interface{})
	require.True(suite.T(), ok)

	// Validate time fields
	assert.Contains(suite.T(), result, "timestamp")
	assert.Contains(suite.T(), result, "formatted")
	assert.Contains(suite.T(), result, "unix")
	assert.Contains(suite.T(), result, "timezone")
}

// TestAPI_StatusValidation tests the status API
func (suite *IntegrationTestSuite) TestAPI_StatusValidation() {
	request := types.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "status",
		ID:      "status-test",
	}

	response := suite.makeHTTPRequest(request)

	assert.Nil(suite.T(), response.Error)
	assert.NotNil(suite.T(), response.Result)

	result, ok := response.Result.(map[string]interface{})
	require.True(suite.T(), ok)

	// Validate status fields
	assert.Contains(suite.T(), result, "status")
	assert.Contains(suite.T(), result, "version")
	assert.Contains(suite.T(), result, "uptime")
}
