#!/bin/bash

# Docker Compose integration test runner
# Runs tests in Docker Compose environment

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${PROJECT_ROOT}/docker-compose.test.yml"
TIMEOUT=${TEST_TIMEOUT:-300}
QUIET_MODE=${QUIET_TESTS:-0}
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

# Check Docker and Docker Compose
check_docker() {
    if ! command -v docker >/dev/null 2>&1; then
        log_error "❌ Docker is not installed or not in PATH"
        exit 1
    fi
    
    if ! command -v docker-compose >/dev/null 2>&1; then
        log_error "❌ Docker Compose is not installed or not in PATH"
        exit 1
    fi
    
    log_info "✅ Docker and Docker Compose are available"
}

# Check if Docker Compose file exists
check_compose_file() {
    if [[ ! -f "${COMPOSE_FILE}" ]]; then
        log_error "❌ Docker Compose file not found: ${COMPOSE_FILE}"
        exit 1
    }
    
    log_info "✅ Docker Compose file found: ${COMPOSE_FILE}"
}

# Start Docker Compose services
start_services() {
    log_header "🚀 Starting Docker Compose services"
    
    # Set environment variables for Docker Compose
    export TEST_TIMEOUT="${TIMEOUT}"
    export QUIET_TESTS="${QUIET_MODE}"
    export DOCKER_COMPOSE_TEST=1
    export TEST_ISOLATION=docker
    
    # Start services
    log_info "Starting services defined in ${COMPOSE_FILE}"
    if ! docker-compose -f "${COMPOSE_FILE}" up -d; then
        log_error "❌ Failed to start Docker Compose services"
        exit 1
    }
    
    log_success "✅ Docker Compose services started"
    
    # Show running containers
    if [[ "${QUIET_MODE}" != "1" ]]; then
        log_info "📊 Running containers:"
        docker-compose -f "${COMPOSE_FILE}" ps
    fi
}

# Run tests in Docker Compose environment
run_tests() {
    log_header "🧪 Running tests in Docker Compose environment"
    
    # Set environment variables for test client
    export TEST_TIMEOUT="${TIMEOUT}"
    export QUIET_TESTS="${QUIET_MODE}"
    export DOCKER_COMPOSE_TEST=1
    export TEST_ISOLATION=docker
    
    # Build test command
    local test_cmd="cd /app && go test -v -timeout ${TIMEOUT}s ./test/integration/... -run TestIntegrationSuite"
    
    # Add failfast flag if bail is enabled
    if [[ "${BAIL_ON_FAILURE}" == "true" ]]; then
        test_cmd="${test_cmd} -failfast"
        log_info "🚨 Bail mode enabled - tests will stop on first failure"
    fi
    
    # Run tests in test-client container
    log_info "Executing tests in test-client container"
    log_info "Command: ${test_cmd}"
    
    if ! docker-compose -f "${COMPOSE_FILE}" exec -T test-client bash -c "${test_cmd}"; then
        log_error "❌ Tests failed"
        return 1
    }
    
    log_success "✅ All tests passed"
    return 0
}

# Stop Docker Compose services
stop_services() {
    log_header "🛑 Stopping Docker Compose services"
    
    # Stop and remove containers
    log_info "Stopping and removing containers"
    if ! docker-compose -f "${COMPOSE_FILE}" down; then
        log_warning "⚠️  Failed to stop some Docker Compose services"
    }
    
    log_success "✅ Docker Compose services stopped"
}

# Cleanup function
cleanup() {
    local exit_code=$?
    
    # Always try to stop services on exit
    if [[ "${KEEP_RUNNING:-0}" != "1" ]]; then
        stop_services
    else
        log_info "🔄 Keeping Docker Compose services running as requested"
    fi
    
    if [[ $exit_code -eq 0 ]]; then
        log_success "🎉 Docker integration test execution completed successfully"
    else
        log_warning "⚠️  Docker integration test execution completed with errors"
    fi
    
    exit $exit_code
}

# Main execution
main() {
    log_header "🐳 Docker Integration Test Runner"
    log_info "📁 Project root: ${PROJECT_ROOT}"
    log_info "📄 Compose file: ${COMPOSE_FILE}"
    log_info "⏱️  Test timeout: ${TIMEOUT}s"
    log_info "🤫 Quiet mode: $([ "${QUIET_MODE}" == "1" ] && echo "enabled" || echo "disabled")"
    log_info "🚨 Bail on failure: $([ "${BAIL_ON_FAILURE}" == "true" ] && echo "enabled" || echo "disabled")"
    
    # Check prerequisites
    check_docker
    check_compose_file
    
    # Start services
    start_services
    
    # Run tests
    run_tests
}

# Help function
show_help() {
    cat << EOF
Docker Compose Integration Test Runner

This script runs integration tests in a Docker Compose environment.

Usage: $0 [OPTIONS]

OPTIONS:
    -h, --help          Show this help message
    -q, --quiet         Run in quiet mode (minimal output)
    -t, --timeout SEC   Set test timeout in seconds (default: 300)
    -k, --keep-running  Keep Docker Compose services running after tests
    -b, --bail          Stop on first test failure (failfast mode)

ENVIRONMENT VARIABLES:
    TEST_TIMEOUT        Test timeout in seconds
    QUIET_TESTS         Set to 1 for quiet mode
    KEEP_RUNNING        Set to 1 to keep services running after tests

EXAMPLES:
    $0                          Run tests with default settings
    $0 --quiet                  Run tests in quiet mode
    $0 --timeout 600            Run tests with 10-minute timeout
    $0 --keep-running           Keep Docker Compose services running after tests
    $0 --bail                   Stop on first test failure
    
    QUIET_TESTS=1 $0            Run tests quietly
    TEST_TIMEOUT=600 $0         Run tests with 10-minute timeout
    KEEP_RUNNING=1 $0           Keep services running after tests

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
            TIMEOUT="$2"
            shift 2
            ;;
        -k|--keep-running)
            KEEP_RUNNING=1
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
if ! [[ "$TIMEOUT" =~ ^[0-9]+$ ]] || [[ "$TIMEOUT" -lt 30 ]]; then
    log_error "❌ Invalid timeout value: $TIMEOUT (must be >= 30 seconds)"
    exit 1
fi

# Run main function
main "$@"
