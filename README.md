# SimpleHTTPRedirector

A lightweight, high-performance HTTP redirect service with JSON-based configuration. Perfect for managing domain redirects in containerized environments like Coolify, or behind reverse proxies like Traefik.

## Features

- **JSON Configuration** - Simple, declarative redirect rules without environment variable complexity
- **Multiple Sources per Rule** - Redirect multiple domains to a single target
- **Flexible Redirect Types** - Support for `permanent` (301), `temporary` (302), or any 3xx status code
- **Path & Query Preservation** - Optionally preserve URL paths and query parameters
- **Wildcard Support** - Match subdomains with `*.domain.com` patterns
- **Auto www Handling** - Rules for `domain.com` automatically match `www.domain.com`
- **Proxy-Aware** - Correctly handles `X-Forwarded-Host` headers from reverse proxies
- **Trusted Proxy Mode** - Configurable trust for proxy headers (`TRUST_PROXY`)
- **Rate Limiting** - Built-in protection against request flooding
- **Secure by Default** - Target URL validation prevents open redirect vulnerabilities
- **Lightweight** - ~30MB Docker image based on Alpine Linux
- **Health Endpoint** - Built-in `/health` endpoint for container orchestration

## Quick Start

### Using Docker Compose (Coolify)

```bash
# Clone the repository
git clone https://github.com/bauer-group/CS-SimpleHTTPRedirector.git
cd CS-SimpleHTTPRedirector

# Create your configuration
cp config/redirects.example.json config/redirects.json
# Edit config/redirects.json with your redirect rules

# Start the service
docker compose up -d
```

### Using Docker Compose (Traefik)

```bash
# Copy environment template
cp .env.example .env
# Edit .env with your settings

# Start with Traefik labels
docker compose -f docker-compose.traefik.yml up -d
```

### Local Development

```bash
# Build and run with port mapping
docker compose -f docker-compose.development.yml up --build

# Test a redirect
curl -I -H "Host: example.com" http://localhost:8080/
```

## Configuration

### Redirect Rules (`config/redirects.json`)

The configuration file is a JSON array of redirect rules:

```json
[
  {
    "source": ["old-domain.com", "www.old-domain.com"],
    "target": "https://new-domain.com",
    "type": "permanent",
    "preserve_path": true,
    "preserve_query": true
  },
  {
    "source": ["campaign.example.com"],
    "target": "https://shop.example.com/promo",
    "type": "temporary",
    "preserve_path": false,
    "preserve_query": true
  },
  {
    "source": ["*.legacy.com"],
    "target": "https://modern.com",
    "type": "308",
    "preserve_path": true,
    "preserve_query": false
  }
]
```

### Rule Properties

| Property | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| `source` | `string[]` | Yes | - | Array of source hostnames to match |
| `target` | `string` | Yes | - | Target URL to redirect to |
| `type` | `string` | No | `permanent` | Redirect type (see below) |
| `preserve_path` | `boolean` | No | `false` | Append original path to target |
| `preserve_query` | `boolean` | No | `false` | Append query parameters to target |

### Redirect Types

| Type | HTTP Status | Use Case |
|------|-------------|----------|
| `permanent` | 301 | Permanent domain moves (SEO-friendly) |
| `temporary` | 302 | Temporary redirects, A/B testing |
| `303` | 303 | Redirect after POST (See Other) |
| `307` | 307 | Temporary redirect preserving method |
| `308` | 308 | Permanent redirect preserving method |

### Source Matching

Sources are matched in the following order:

1. **Exact match** - `docs.example.com` matches `docs.example.com`
2. **Wildcard match** - `*.example.com` matches `anything.example.com`
3. **www handling** - `example.com` also matches `www.example.com` (and vice versa)

### Path & Query Preservation

**Example with `preserve_path: true`:**
```
source.com/products/item-123 → target.com/products/item-123
```

**Example with `preserve_query: true`:**
```
source.com?ref=campaign&id=42 → target.com?ref=campaign&id=42
```

**Example with target path and preservation:**
```json
{
  "source": ["old-docs.com"],
  "target": "https://new-site.com/documentation/",
  "preserve_path": true
}
```
```
old-docs.com/api/v2 → new-site.com/documentation/api/v2
```

## Docker Compose Files

| File | Purpose | Network |
|------|---------|---------|
| `docker-compose.yml` | Coolify deployment | Coolify-managed |
| `docker-compose.traefik.yml` | Standalone with Traefik | External `proxy` network |
| `docker-compose.development.yml` | Local development | Host port 8080 |

## Environment Variables

Copy `.env.example` to `.env` and configure as needed:

```bash
# Server settings (all compose files)
TRUST_PROXY=true               # Trust X-Forwarded-* headers (default: true)

# For docker-compose.traefik.yml
STACK_NAME=redirector          # Container name prefix
PROXY_NETWORK=proxy            # External Traefik network
CONFIG_PATH=./config           # Config directory path
REDIRECT_HOST_RULE=Host(`...`) # Traefik routing rule

# For docker-compose.development.yml
HOST_PORT=8080                 # Local port mapping
```

### Security Settings

| Variable | Default | Description |
| --- | --- | --- |
| `TRUST_PROXY` | `true` | Trust `X-Forwarded-*` headers from reverse proxy. Set to `false` if exposed directly to the internet. |

## Health Check

The service exposes a health endpoint at `/health`:

```bash
curl http://localhost:8080/health
# {"status":"healthy"}
```

## Logging

All redirects are logged with source, target, and status code:

```
2025/01/15 10:30:45 Loaded 3 redirect rules
2025/01/15 10:30:45   [old-domain.com www.old-domain.com] -> https://new-domain.com (301, path=true, query=true)
2025/01/15 10:30:45 Starting HTTP Redirector on port 8080
2025/01/15 10:31:02 Redirecting old-domain.com/page -> https://new-domain.com/page (301)
```

## Coolify Deployment

1. Add repository as a Docker Compose project
2. Select `docker-compose.yml`
3. Configure domains in Coolify's GUI (add all source domains)
4. Mount config volume or use Coolify's file manager to edit `config/redirects.json`
5. Deploy

## Building

```bash
# Build the Docker image
docker build -t simple-http-redirector:latest ./src

# Build with specific Go version
docker build --build-arg GO_VERSION=1.25 -t simple-http-redirector:latest ./src
```

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Client        │────▶│  Traefik/Proxy  │────▶│  Redirector     │
│                 │     │                 │     │                 │
│                 │◀────│  X-Forwarded-*  │◀────│  301/302/3xx    │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                        │
                                                        ▼
                                                ┌─────────────────┐
                                                │ redirects.json  │
                                                └─────────────────┘
```

## License

MIT License - see [LICENSE](LICENSE) for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Submit a pull request

## Support

- Issues: [GitHub Issues](https://github.com/bauer-group/CS-SimpleHTTPRedirector/issues)
- Documentation: [GitHub Wiki](https://github.com/bauer-group/CS-SimpleHTTPRedirector/wiki)
