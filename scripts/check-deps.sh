#!/bin/bash
# Script to check dependencies for the Go Streaming Server project

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}Checking dependencies for Go Streaming Server...${NC}"

# Check Go version
MIN_GO_VERSION="1.18"
if ! command -v go &> /dev/null; then
    echo -e "${RED}Go is not installed. Please install Go $MIN_GO_VERSION or higher.${NC}"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
if [ "$(printf '%s\n' "$MIN_GO_VERSION" "$GO_VERSION" | sort -V | head -n1)" != "$MIN_GO_VERSION" ]; then
    echo -e "${RED}Go version $MIN_GO_VERSION or higher is required. Installed version: $GO_VERSION${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Go version: $(go version)${NC}"

# Check Docker
MIN_DOCKER_VERSION="20.10.0"
if ! command -v docker &> /dev/null; then
    echo -e "${YELLOW}Docker is not installed. Some features may not work.${NC}"
else
    DOCKER_VERSION=$(docker --version | awk '{print $3}' | sed 's/,//')
    if [ "$(printf '%s\n' "$MIN_DOCKER_VERSION" "$DOCKER_VERSION" | sort -V | head -n1)" != "$MIN_DOCKER_VERSION" ]; then
        echo -e "${YELLOW}Docker version $MIN_DOCKER_VERSION or higher is recommended. Installed version: $DOCKER_VERSION${NC}"
    else
        echo -e "${GREEN}✓ Docker version: $(docker --version)${NC}"
    fi
fi

# Check Docker Compose
MIN_COMPOSE_VERSION="2.0.0"
if ! command -v docker-compose &> /dev/null; then
    if ! docker compose version &> /dev/null; then
        echo -e "${YELLOW}Docker Compose is not installed. Some features may not work.${NC}"
    else
        echo -e "${GREEN}✓ Docker Compose: $(docker compose version)${NC}"
    fi
else
    echo -e "${GREEN}✓ Docker Compose: $(docker-compose --version)${NC}"
fi

# Check required tools
for tool in git curl make; do
    if ! command -v $tool &> /dev/null; then
        echo -e "${RED}$tool is not installed. Please install $tool.${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ $tool installed: $($tool --version | head -n1)${NC}"
done

# Check Go tools
echo -e "${BLUE}Checking Go tools...${NC}"

# Check golangci-lint
if ! command -v golangci-lint &> /dev/null; then
    echo -e "${YELLOW}golangci-lint is not installed. It will be installed when needed.${NC}"
else
    echo -e "${GREEN}✓ golangci-lint installed: $(golangci-lint --version)${NC}"
fi

# Check goimports
if ! command -v goimports &> /dev/null; then
    echo -e "${YELLOW}goimports is not installed. It will be installed when needed.${NC}"
else
    echo -e "${GREEN}✓ goimports installed${NC}"
fi

# Check mockgen
if ! command -v mockgen &> /dev/null; then
    echo -e "${YELLOW}mockgen is not installed. It will be installed when needed.${NC}"
else
    echo -e "${GREEN}✓ mockgen installed${NC}"
fi

# Check environment
if [ ! -f .env ]; then
    echo -e "${YELLOW}.env file not found. Creating from .env.example...${NC}"
    cp .env.example .env 2>/dev/null || echo "# Environment variables" > .env
fi
echo -e "${GREEN}✓ .env file exists${NC}"

# Check GOPATH
if [ -z "$GOPATH" ]; then
    echo -e "${YELLOW}GOPATH is not set. Using default value.${NC}"
else
    echo -e "${GREEN}✓ GOPATH: $GOPATH${NC}"
fi

# Check module dependencies
echo -e "${BLUE}Checking module dependencies...${NC}"
if ! go mod verify &> /dev/null; then
    echo -e "${YELLOW}Module dependencies have issues. Running go mod tidy...${NC}"
    go mod tidy
else
    echo -e "${GREEN}✓ Module dependencies verified${NC}"
fi

# Check for available ports
echo -e "${BLUE}Checking for available ports...${NC}"
for port in 8080 8443 8081 8082 18080 18081 18082; do
    if lsof -Pi :$port -sTCP:LISTEN -t &> /dev/null; then
        echo -e "${YELLOW}Port $port is already in use. This may cause issues when running the server or tests.${NC}"
    else
        echo -e "${GREEN}✓ Port $port is available${NC}"
    fi
done

echo -e "${GREEN}Dependency check completed!${NC}"
exit 0
