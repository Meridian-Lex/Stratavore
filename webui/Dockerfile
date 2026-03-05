# Stratavore WebUI Container
# Lightweight Python HTTP server for serving the modular control plane
FROM python:3.14-alpine

LABEL maintainer="Meridian Lex <lex@meridian-ops>"
LABEL description="Stratavore Job Tracker WebUI v4 - Browser-based control plane"

# Install runtime dependencies
RUN apk add --no-cache \
    wget \
    && rm -rf /var/cache/apk/*

# Create application directory
WORKDIR /app

# Copy WebUI files
COPY . /app/

# Create non-root user for security
RUN addgroup -g 1000 stratavore && \
    adduser -D -u 1000 -G stratavore stratavore && \
    chown -R stratavore:stratavore /app

# Switch to non-root user
USER stratavore

# Expose configurable port (default 8080)
EXPOSE 8080

# Environment variables for runtime configuration
ENV PORT=8080
ENV HOST=0.0.0.0
ENV DAEMON_API_URL=http://stratavore-daemon:8080

# Health check endpoint
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:${PORT}/index.html || exit 1

# Run the modular HTTP server
ENTRYPOINT ["python3", "server.py"]
CMD ["--port", "8080", "--host", "0.0.0.0"]
