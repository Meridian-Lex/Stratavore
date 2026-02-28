# Stratavore WebUI Deployment Guide

## Overview

The Stratavore WebUI is containerized and integrated into the main docker-compose stack with nginx reverse proxy for unified ingress.

## Architecture

```
Browser (http://localhost:8080)
    |
    v
nginx Reverse Proxy (port 80 internal)
    |
    +---> /api/*        -> stratavore-daemon:8080 (HTTP API)
    +---> /             -> stratavore-webui:8080 (Static files + WebUI API)
```

## Components

### 1. WebUI Container (`webui/Dockerfile`)
- **Base Image**: `python:3.11-alpine`
- **Size**: 58.3 MB
- **Exposed Port**: 8080 (internal only)
- **Server**: Python HTTP server via `server.py`
- **Health Check**: `wget --spider http://localhost:8080/index.html`
- **Environment Variables**:
  - `PORT`: Server port (default 8080)
  - `HOST`: Bind address (default 0.0.0.0)
  - `DAEMON_API_URL`: Daemon API endpoint (default http://stratavore-daemon:8080)

### 2. nginx Reverse Proxy (`deployments/nginx.conf`)
- **Base Image**: `nginx:1.25-alpine`
- **Public Port**: 8080 (host) -> 80 (container)
- **Upstream Services**:
  - `daemon_api`: stratavore-daemon:8080
  - `webui`: stratavore-webui:8080
- **Features**:
  - Gzip compression
  - Static asset caching (1 day)
  - CORS headers
  - Security headers (X-Frame-Options, X-Content-Type-Options, X-XSS-Protection)
  - Health check endpoint (`/health`)

### 3. docker-compose Integration
- WebUI depends on `stratavore-daemon` (health check)
- nginx depends on both `stratavore-daemon` and `webui`
- All services use the `stratavore` network
- Daemon API port 8080 is no longer publicly exposed (internal only via nginx proxy)

## Deployment

### Production Deployment

```bash
# Build and launch full stack
docker-compose up --build

# Access WebUI
open http://localhost:8080

# Check service health
curl http://localhost:8080/health        # nginx
curl http://localhost:8080/api/v1/health # daemon (via proxy)
```

### Testing Deployment

A minimal test stack is provided for integration testing without full infrastructure dependencies:

```bash
# Launch test stack (mock daemon, webui, nginx)
docker-compose -f docker-compose.test.yml up -d

# Access test WebUI
open http://localhost:8888

# Shut down test stack
docker-compose -f docker-compose.test.yml down
```

Test stack includes:
- `mock-daemon`: Minimal HTTP server simulating daemon API endpoints
- `webui`: Full WebUI container (production-identical)
- `nginx`: Reverse proxy with test configuration (`nginx.test.conf`)

## Health Checks

All services include Docker health checks:

```bash
# Check all service health statuses
docker ps --format "table {{.Names}}\t{{.Status}}"

# Inspect specific service health
docker inspect stratavore-nginx --format '{{json .State.Health}}' | jq .
docker inspect stratavore-webui --format '{{json .State.Health}}' | jq .
```

## Logs

```bash
# View WebUI logs
docker logs stratavore-webui

# View nginx logs
docker logs stratavore-nginx

# Follow all logs
docker-compose logs -f webui nginx
```

## Security

- WebUI runs as non-root user (`stratavore:stratavore`, UID/GID 1000)
- nginx includes standard security headers
- Internal services (daemon, webui) are NOT exposed to host network (only via nginx proxy)

## File Structure

```
.
├── webui/
│   ├── Dockerfile              # WebUI container definition
│   ├── server.py               # Python HTTP server
│   ├── index.html              # Main UI file
│   ├── components/             # UI components
│   ├── services/               # Frontend services
│   └── styles/                 # CSS styles
├── deployments/
│   ├── nginx.conf              # Production nginx config
│   ├── nginx.test.conf         # Test nginx config
│   └── mock-daemon.py          # Mock daemon for testing
├── docker-compose.yml          # Production stack
└── docker-compose.test.yml     # Test stack (minimal dependencies)
```

## Troubleshooting

### Port Already Allocated

If you see "port 8080 already allocated":
```bash
# Find conflicting container
docker ps | grep 8080

# Stop conflicting services
docker-compose down
```

### WebUI Not Loading

```bash
# Check WebUI health
docker logs stratavore-webui

# Verify nginx routing
curl -I http://localhost:8080/index.html
```

### API Calls Failing

```bash
# Check daemon health
curl http://localhost:8080/api/v1/health

# Check nginx proxy configuration
docker exec stratavore-nginx cat /etc/nginx/nginx.conf

# Test direct connection (bypass nginx)
docker exec stratavore-nginx wget -qO- http://stratavore-daemon:8080/api/v1/health
```

## Performance

- **WebUI Container**: <60 MB, Alpine-based
- **nginx**: Minimal overhead, keepalive connections enabled
- **Static Asset Caching**: 1-day cache for JS/CSS/images
- **Gzip Compression**: Enabled for text-based content

## Next Steps

1. Enable HTTPS via Let's Encrypt or custom certificates
2. Add rate limiting to nginx config
3. Implement WebSocket support for real-time updates
4. Add monitoring/metrics for nginx (Prometheus exporter)
