#!/bin/bash
# =============================================================================
# SimpleHTTPRedirector - Tools Container Launcher (Linux/macOS)
# =============================================================================

set -euo pipefail

# Configuration
TOOLS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$TOOLS_DIR")"
IMAGE_NAME="redirector-tools"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Check if Docker is running
if ! docker info &> /dev/null; then
    echo -e "${RED}[ERROR] Docker is not running. Please start Docker first.${NC}"
    exit 1
fi

# Show help
show_help() {
    echo "Usage: $0 [COMMAND] [OPTIONS]"
    echo ""
    echo "Commands:"
    echo "  generate-env     Generate .env from redirects.json"
    echo "  shell            Start interactive shell in tools container"
    echo ""
    echo "Options:"
    echo "  --build, -b      Rebuild the tools container"
    echo "  --help, -h       Show this help"
    echo ""
    echo "Examples:"
    echo "  $0 generate-env              Generate .env file"
    echo "  $0 generate-env --build      Rebuild container and generate .env"
    echo "  $0 shell                     Start interactive shell"
    exit 0
}

# Parse arguments
COMMAND=""
BUILD=false

while [[ $# -gt 0 ]]; do
    case $1 in
        generate-env|shell)
            COMMAND="$1"
            shift
            ;;
        --build|-b)
            BUILD=true
            shift
            ;;
        --help|-h)
            show_help
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            show_help
            ;;
    esac
done

# Default to help if no command
if [[ -z "$COMMAND" ]]; then
    show_help
fi

# Build if requested or image doesn't exist
if [[ "$BUILD" == "true" ]] || ! docker image inspect "$IMAGE_NAME" &> /dev/null; then
    echo -e "${CYAN}[INFO] Building tools container...${NC}"
    docker build -t "$IMAGE_NAME" "$TOOLS_DIR"
    echo ""
fi

# Execute command
case $COMMAND in
    generate-env)
        echo -e "${CYAN}[INFO] Generating .env from redirects.json...${NC}"
        docker run --rm \
            -v "$PROJECT_DIR:/workspace" \
            -w /workspace \
            "$IMAGE_NAME" \
            /bin/bash /workspace/scripts/generate-env.sh
        ;;
    shell)
        echo -e "${GREEN}===========================================${NC}"
        echo -e "${GREEN} SimpleHTTPRedirector Tools${NC}"
        echo -e "${GREEN}===========================================${NC}"
        echo ""
        echo "Available scripts:"
        echo -e "  ${YELLOW}./scripts/generate-env.sh${NC}  - Generate .env from config"
        echo ""
        echo "Type 'exit' to leave the container."
        echo -e "${GREEN}===========================================${NC}"
        echo ""
        docker run -it --rm \
            -v "$PROJECT_DIR:/workspace" \
            -w /workspace \
            "$IMAGE_NAME"
        ;;
esac
