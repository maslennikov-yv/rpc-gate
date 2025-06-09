#!/bin/bash

# Enhanced integration test runner with automatic environment detection
# Supports both local development and Docker Compose environments

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TEST_TIMEOUT=${TEST_TIMEOUT:-300}
QUIET_MODE=${QUIET_TESTS:-0}
FORCE_DOCKER=${DOCKER_COMPOSE_TEST:-0}
FORCE_LOCAL=${LOCAL_TEST:-0}
BAIL_ON_FAILURE=false

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    if [[ "${QUIET_MODE}" != "1" ]]; then
        echo -e "${BLUE}[INFO]${NC} $1"
    fi
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_header() {
    if [[ "${QUIET_MODE}" != "1" ]]; then
        echo -e "${CYAN}$1${NC}"
    fi
}

# Environment detection
detect_environment() {
    # Store detection reasons
    local reasons=()
    
    # Default to local environment
    local env_type="local"
    
    # Check for Docker environment indicators
    if [[ "${FORCE_DOCKER}" == "1" ]]; then
        env_type="docker"
        reasons+=("DOCKER_COMPOSE_TEST=1 (forced)")
    elif [[ "${FORCE_LOCAL}" == "1" ]]; then
        env_type="local"
        reasons+=("LOCAL_TEST=1 (forced)")
    elif [[ -n "${TEST_SERVER_HTTP:-}" ]]; then
        env_type="docker"
        reasons+=("TEST_SERVER_HTTP environment variable set")
    elif [[ -f "/.dockerenv" ]]; then
        env_type="docker"
        reasons+=("Running inside Docker container")
    elif docker-compose -f "${PROJECT_ROOT}/docker-compose.test.yml" ps 2>/dev/null | grep -q "streaming-server-test"; then
        env_type="docker"
        reasons+=("Docker Compose services are running")
    elif ! command -v go >/dev/null 2>&1; then
        if command -v docker >/dev/null 2>&1; then
            env_type="docker"
            reasons+=("Go not available, Docker available")
        else
            log_error "❌ Neither Go nor Docker is available"
            exit 1
        fi
    fi
    
    # Store reasons in global variable for later use
    DETECTION_REASONS=("${reasons[@]}")
    
    # Return environment type
    echo "${env_type}"
}

# Run tests in Docker environment
run_docker_tests() {
    log_header "🐳 Running Integration Tests in Docker Environment"
    
    # Use the dedicated Docker test script
    local docker_script="${SCRIPT_DIR}/docker-test.sh"
    
    if [[ ! -f "${docker_script}" ]]; then
        log_error "❌ Docker test script not found: ${docker_script}"
        exit 1
    fi
    
    # Pass through relevant environment variables
    local docker_env_args=()
    if [[ "${QUIET_MODE}" == "1" ]]; then
        docker_env_args+=("--quiet")
    fi
    if [[ -n "${TEST_TIMEOUT:-}" ]]; then
        docker_env_args+=("--timeout" "${TEST_TIMEOUT}")
    fi
    if [[ "${BAIL_ON_FAILURE}" == "true" ]]; then
        docker_env_args+=("--bail")
    fi
    
    log_info "🚀 Delegating to Docker test runner..."
    exec "${docker_script}" "${docker_env_args[@]}"
}

# Run tests in local environment
run_local_tests() {
    log_header "🖥️  Running Integration Tests in Local Environment"
    
    cd "${PROJECT_ROOT}"
    
    # Check Go installation
    if ! command -v go >/dev/null 2>&1; then
        log_error "❌ Go is not installed or not in PATH"
        exit 1
    fi
    
    # Check Go version
    local go_version
    go_version=$(go version | awk '{print $3}' | sed 's/go//')
    log_info "🔧 Go version: ${go_version}"
    
    # Ensure dependencies are available
    log_info "📦 Checking Go dependencies..."
    if ! go mod tidy; then
        log_error "❌ Failed to resolve Go dependencies"
        exit 1
    fi
    
    # Clean up test environment (only in local mode)
    log_info "🧹 Cleaning up test environment..."
    if ! "${SCRIPT_DIR}/cleanup-test-processes.sh" --quiet; then
        log_warning "⚠️  Cleanup completed with warnings"
    fi
    
    # Run tests with appropriate flags
    log_info "🧪 Running integration tests locally..."
    
    local test_args=(
        "-v"
        "-timeout" "${TEST_TIMEOUT}s"
        "./test/integration/..."
        "-run" "TestIntegrationSuite"
    )
    
    # Add failfast flag if bail is enabled
    if [[ "${BAIL_ON_FAILURE}" == "true" ]]; then
        test_args=("-failfast" "${test_args[@]}")
        log_info "🚨 Bail mode enabled - tests will stop on first failure"
    fi
    
    # Set environment variables for local testing
    export TEST_ISOLATION=local
    export QUIET_TESTS=${QUIET_MODE}
    
    if [[ "${QUIET_MODE}" == "1" ]]; then
        # In quiet mode, capture output and only show on failure
        local temp_output
        temp_output=$(mktemp)
        
        if go test "${test_args[@]}" > "${temp_output}" 2>&1; then
            log_success "✅ All integration tests passed!"
            rm -f "${temp_output}"
            return 0
        else
            log_error "❌ Integration tests failed!"
            log_info "📋 Test output:"
            cat "${temp_output}"
            rm -f "${temp_output}"
            return 1
        fi
    else
        # Normal mode - show output directly
        if go test "${test_args[@]}"; then
            log_success "✅ All integration tests passed!"
            return 0
        else
            log_error "❌ Integration tests failed!"
            return 1
        fi
    fi
}

# Cleanup function
cleanup() {
    local exit_code=$?
    
    # Only perform cleanup in local mode and if not already cleaning up
    if [[ "${ENV_TYPE:-local}" == "local" ]] && [[ "${CLEANUP_IN_PROGRESS:-0}" != "1" ]]; then
        export CLEANUP_IN_PROGRESS=1
        if [[ "${QUIET_MODE}" != "1" ]]; then
            log_info "🧹 Performing final cleanup..."
        fi
        "${SCRIPT_DIR}/cleanup-test-processes.sh" --quiet || true
    fi
    
    if [[ $exit_code -eq 0 ]]; then
        log_success "🎉 Integration test execution completed successfully"
    else
        log_warning "⚠️  Integration test execution completed with errors"
    fi
    
    exit $exit_code
}

# Main execution
main() {
    log_header "🧪 Integration Test Runner"
    log_info "📁 Project root: ${PROJECT_ROOT}"
    log_info "⏱️  Test timeout: ${TEST_TIMEOUT}s"
    log_info "🤫 Quiet mode: $([ "${QUIET_MODE}" == "1" ] && echo "enabled" || echo "disabled")"
    log_info "🚨 Bail on failure: $([ "${BAIL_ON_FAILURE}" == "true" ] && echo "enabled" || echo "disabled")"
    
    # Initialize global variables
    DETECTION_REASONS=()
    
    # Detect environment
    ENV_TYPE=$(detect_environment)
    
    # Log environment detection
    log_info "🔍 Environment detection: ${ENV_TYPE}"
    for reason in "${DETECTION_REASONS[@]}"; do
        log_info "   - ${reason}"
    done
    
    # Run tests based on environment
    case "${ENV_TYPE}" in
        docker)
            run_docker_tests
            ;;
        local)
            run_local_tests
            ;;
        *)
            log_error "❌ Unknown environment type: ${ENV_TYPE}"
            exit 1
            ;;
    esac
}

# Help function
show_help() {
    cat << EOF
Integration Test Runner with Automatic Environment Detection

This script automatically detects whether to run tests in Docker Compose
or local development environment and executes them appropriately.

Usage: $0 [OPTIONS]

OPTIONS:
    -h, --help          Show this help message
    -q, --quiet         Run in quiet mode (minimal output)
    -t, --timeout SEC   Set test timeout in seconds (default: 300)
    -d, --docker        Force Docker Compose environment
    -l, --local         Force local development environment
    -b, --bail          Stop on first test failure (failfast mode)

ENVIRONMENT VARIABLES:
    TEST_TIMEOUT        Test timeout in seconds
    QUIET_TESTS         Set to 1 for quiet mode
    DOCKER_COMPOSE_TEST Set to 1 to force Docker environment
    LOCAL_TEST          Set to 1 to force local environment

AUTOMATIC DETECTION:
    The script automatically detects the environment based on:
    - Environment variables (TEST_SERVER_HTTP, etc.)
    - Docker container indicators (/.dockerenv)
    - Running Docker Compose services
    - Available tools (Go, Docker)

EXAMPLES:
    $0                          Auto-detect and run tests
    $0 --quiet                  Run tests in quiet mode
    $0 --docker                 Force Docker Compose environment
    $0 --local                  Force local development environment
    $0 --timeout 600            Run tests with 10-minute timeout
    $0 --bail                   Stop on first test failure
    
    DOCKER_COMPOSE_TEST=1 $0    Force Docker environment
    LOCAL_TEST=1 $0             Force local environment
    QUIET_TESTS=1 $0            Run tests quietly

EOF
}

# Set up cleanup trap
trap cleanup EXIT INT TERM

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            exit 0
            ;;
        -q|--quiet)
            QUIET_MODE=1
            export QUIET_TESTS=1
            shift
            ;;
        -t|--timeout)
            TEST_TIMEOUT="$2"
            shift 2
            ;;
        -d|--docker)
            FORCE_DOCKER=1
            export DOCKER_COMPOSE_TEST=1
            shift
            ;;
        -l|--local)
            FORCE_LOCAL=1
            export LOCAL_TEST=1
            shift
            ;;
        -b|--bail)
            BAIL_ON_FAILURE=true
            shift
            ;;
        *)
            log_error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Validate arguments
if [[ "${FORCE_DOCKER}" == "1" && "${FORCE_LOCAL}" == "1" ]]; then
    log_error "❌ Cannot force both Docker and local environments"
    exit 1
fi

if ! [[ "$TEST_TIMEOUT" =~ ^[0-9]+$ ]] || [[ "$TEST_TIMEOUT" -lt 30 ]]; then
    log_error "❌ Invalid timeout value: $TEST_TIMEOUT (must be >= 30 seconds)"
    exit 1
fi

# Run main function
main "$@"
