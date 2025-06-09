#!/bin/bash

# Test cleanup script
# Cleans up test processes and ports

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
QUIET_MODE=${QUIET_TESTS:-0}
FORCE_CLEANUP=${FORCE_CLEANUP:-0}

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
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

# Check if running in Docker environment
is_docker_environment() {
    if [[ "${DOCKER_COMPOSE_TEST:-0}" == "1" ]] || [[ -f "/.dockerenv" ]] || [[ -n "${TEST_SERVER_HTTP:-}" ]]; then
        return 0
    fi
    return 1
}

# Main cleanup function
main() {
    # Skip cleanup in Docker environment
    if is_docker_environment; then
        if [[ "${QUIET_MODE}" != "1" ]]; then
            log_info "🐳 Docker environment detected - skipping host cleanup"
        fi
        exit 0
    fi
    
    if [[ "${QUIET_MODE}" != "1" ]]; then
        echo "🧹 Очистка тестовых процессов и портов..."
    fi
    
    # Stop test processes
    log_info "Остановка тестовых процессов..."
    
    local processes_found=false
    
    # Find and stop streaming-server test processes
    while IFS= read -r line; do
        if [[ -n "$line" ]]; then
            local pid=$(echo "$line" | awk '{print $2}')
            local cmd=$(echo "$line" | awk '{for(i=11;i<=NF;i++) printf "%s ", $i; print ""}' | sed 's/[[:space:]]*$//')
            
            if [[ -n "$pid" ]] && [[ "$pid" =~ ^[0-9]+$ ]]; then
                log_info "  - Остановка процесса $pid ($cmd)"
                kill -TERM "$pid" 2>/dev/null || true
                processes_found=true
                
                # Wait for graceful shutdown
                local count=0
                while kill -0 "$pid" 2>/dev/null && [[ $count -lt 5 ]]; do
                    sleep 0.5
                    ((count++))
                done
                
                # Force kill if still running
                if kill -0 "$pid" 2>/dev/null; then
                    kill -KILL "$pid" 2>/dev/null || true
                fi
            fi
        fi
    done < <(ps aux | grep -E "(streaming-server|go.*test.*integration)" | grep -v grep | grep -v "$$" || true)
    
    if [[ "$processes_found" == "false" ]]; then
        log_info "Активных тестовых процессов не найдено"
    fi
    
    # Check and free test ports
    log_info "Проверка и освобождение тестовых портов..."
    
    local ports_checked=false
    local start_port=${TEST_PORT_START:-45000}
    local end_port=${TEST_PORT_END:-45100}
    
    for port in $(seq $start_port $end_port); do
        if netstat -tuln 2>/dev/null | grep -q ":$port "; then
            local process_info=$(lsof -ti:$port 2>/dev/null || true)
            if [[ -n "$process_info" ]]; then
                local process_name=$(ps -p "$process_info" -o comm= 2>/dev/null || echo "unknown")
                log_warning "Порт $port занят процессом $process_name (PID: $process_info)"
                ports_checked=true
            fi
        fi
    done
    
    if [[ "$ports_checked" == "false" ]]; then
        log_info "Автоматическая очистка портов не требуется"
    fi
    
    # Clean up temporary files
    log_info "Очистка временных файлов..."
    rm -rf /tmp/streaming-server-test-* 2>/dev/null || true
    rm -rf /tmp/go-test-* 2>/dev/null || true
    
    # Clean up Go test cache
    log_info "Очистка кэша тестов Go..."
    go clean -testcache 2>/dev/null || true
    
    log_success "✅ Очистка завершена"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --quiet)
            QUIET_MODE=1
            shift
            ;;
        --force)
            FORCE_CLEANUP=1
            shift
            ;;
        --help)
            echo "Usage: $0 [--quiet] [--force] [--help]"
            echo "  --quiet  Run in quiet mode"
            echo "  --force  Force cleanup even in Docker environment"
            echo "  --help   Show this help"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Run main function
main "$@"
