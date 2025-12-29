#!/bin/bash
# =============================================================================
# generate-env.sh - Generate .env from redirects.json
#
# Reads the redirect configuration and generates:
# - REDIRECT_HOST_RULE for Traefik
# - Coolify domain list as comment
# - Creates .env from .env.example if not exists
# =============================================================================

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Paths
CONFIG_FILE="/workspace/config/redirects.json"
ENV_FILE="/workspace/.env"
ENV_EXAMPLE="/workspace/.env.example"

# Check if config exists
if [[ ! -f "$CONFIG_FILE" ]]; then
    echo -e "${RED}[ERROR] Config file not found: $CONFIG_FILE${NC}"
    echo "Please create config/redirects.json first."
    exit 1
fi

# Extract all sources from JSON
echo -e "${CYAN}[INFO] Reading sources from redirects.json...${NC}"

# Get all unique sources
SOURCES=$(jq -r '.[].source[]' "$CONFIG_FILE" | sort -u)

if [[ -z "$SOURCES" ]]; then
    echo -e "${RED}[ERROR] No sources found in config${NC}"
    exit 1
fi

# Build REDIRECT_HOST_RULE
echo -e "${CYAN}[INFO] Generating REDIRECT_HOST_RULE...${NC}"
HOST_RULE=""
COOLIFY_DOMAINS=""

while IFS= read -r domain; do
    # Skip empty lines
    [[ -z "$domain" ]] && continue

    # Add to Host rule
    if [[ -n "$HOST_RULE" ]]; then
        HOST_RULE="${HOST_RULE} || "
    fi
    HOST_RULE="${HOST_RULE}Host(\`${domain}\`)"

    # Add to Coolify domains list (https://)
    if [[ -n "$COOLIFY_DOMAINS" ]]; then
        COOLIFY_DOMAINS="${COOLIFY_DOMAINS},"
    fi
    COOLIFY_DOMAINS="${COOLIFY_DOMAINS}https://${domain}"
done <<< "$SOURCES"

# Create .env from template if it doesn't exist
if [[ ! -f "$ENV_FILE" ]]; then
    if [[ -f "$ENV_EXAMPLE" ]]; then
        echo -e "${CYAN}[INFO] Creating .env from .env.example...${NC}"
        cp "$ENV_EXAMPLE" "$ENV_FILE"
    else
        echo -e "${RED}[ERROR] .env.example not found${NC}"
        exit 1
    fi
fi

# Update or add REDIRECT_HOST_RULE in .env
echo -e "${CYAN}[INFO] Updating .env...${NC}"

# Remove existing REDIRECT_HOST_RULE and Coolify comment
sed -i '/^REDIRECT_HOST_RULE=/d' "$ENV_FILE"
sed -i '/^# Coolify Domains:/d' "$ENV_FILE"

# Add new values at the end
cat >> "$ENV_FILE" << EOF

# Coolify Domains: ${COOLIFY_DOMAINS}
REDIRECT_HOST_RULE=${HOST_RULE}
EOF

# Clean up empty lines at end
sed -i -e :a -e '/^\n*$/{$d;N;ba' -e '}' "$ENV_FILE" 2>/dev/null || true

echo ""
echo -e "${GREEN}[SUCCESS] .env updated successfully!${NC}"
echo ""
echo -e "${YELLOW}REDIRECT_HOST_RULE:${NC}"
echo "$HOST_RULE" | fold -s -w 80
echo ""
echo -e "${YELLOW}Coolify Domains (copy to Coolify GUI):${NC}"
echo "$COOLIFY_DOMAINS"
echo ""
