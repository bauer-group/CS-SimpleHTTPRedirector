# =============================================================================
# SimpleHTTPRedirector - Tools Container
# Provides platform-independent tooling for configuration management
# =============================================================================

FROM alpine:latest

# Install required tools
RUN apk add --no-cache \
    bash \
    coreutils \
    jq \
    sed \
    grep

# Set bash as default shell
SHELL ["/bin/bash", "-c"]

# Working directory (will be mounted from host)
WORKDIR /workspace

# Default command
CMD ["/bin/bash"]
