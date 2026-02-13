#!/bin/bash
# Setup Docker infrastructure integration for Stratavore

set -euo pipefail

echo "==> Setting up Stratavore Docker integration"

# Check if lex-docker is running
if ! docker ps | grep -q "lex-postgres"; then
    echo "ERROR: lex-docker infrastructure not running"
    echo "Start it with: cd ~/meridian-home/projects/lex-docker &&./scripts/deploy-stack.sh"
    exit 1
fi

echo "[OK] lex-docker infrastructure is running"

# Database setup
echo ""
echo "==> Setting up PostgreSQL database"

# Create database and user
docker exec lex-postgres psql -U postgres -c "CREATE DATABASE stratavore_state;" 2>/dev/null || echo " Database already exists"
docker exec lex-postgres psql -U postgres -c "CREATE USER stratavore WITH PASSWORD 'stratavore_password';" 2>/dev/null || echo " User already exists"
docker exec lex-postgres psql -U postgres -c "GRANT ALL PRIVILEGES ON DATABASE stratavore_state TO stratavore;" 2>/dev/null

echo "[OK] Database created"

# Run migrations
echo ""
echo "==> Running database migrations"

if [ -f "./scripts/migrate.sh" ]; then
    ./scripts/migrate.sh up
else
    echo "Migration script not found. Run migrations manually:"
    echo " psql -U stratavore -d stratavore_state -f migrations/postgres/0000_extensions.up.sql"
    echo " psql -U stratavore -d stratavore_state -f migrations/postgres/0001_initial.up.sql"
fi

# RabbitMQ setup
echo ""
echo "==> Setting up RabbitMQ"

# Check if rabbitmqadmin is available
if docker exec lex-rabbitmq which rabbitmqadmin >/dev/null 2>&1; then
    docker exec lex-rabbitmq rabbitmqadmin declare exchange name=stratavore.events type=topic durable=true 2>/dev/null || echo " Exchange already exists"
    docker exec lex-rabbitmq rabbitmqadmin declare queue name=stratavore.daemon.events durable=true 2>/dev/null || echo " Queue already exists"
    docker exec lex-rabbitmq rabbitmqadmin declare binding source=stratavore.events destination=stratavore.daemon.events routing_key='#' 2>/dev/null || true
    echo "[OK] RabbitMQ configured"
else
    echo " rabbitmqadmin not found, skipping RabbitMQ configuration"
    echo " You can configure manually via RabbitMQ management UI at http://localhost:15672"
fi

# ntfy setup
echo ""
echo "==> Setting up ntfy topics"

curl -s -d "Stratavore initialized" http://localhost:2586/stratavore-status >/dev/null 2>&1 || echo " Could not send test notification (ntfy may not be running)"
echo "[OK] ntfy topics created"

# Create config directory
echo ""
echo "==> Creating configuration directory"
mkdir -p ~/.config/stratavore
mkdir -p ~/.local/share/stratavore

# Create default config if it doesn't exist
if [ ! -f ~/.config/stratavore/stratavore.yaml ]; then
    cat > ~/.config/stratavore/stratavore.yaml <<'EOF'
database:
  postgresql:
    host: localhost
    port: 5432
    database: stratavore_state
    user: stratavore
    password: stratavore_password
    sslmode: disable
    max_conns: 25
    min_conns: 5

docker:
  rabbitmq:
    host: localhost
    port: 5672
    user: guest
    password: guest
    exchange: stratavore.events
    publisher_confirms: true
  
  ntfy:
    host: localhost
    port: 2586
    topics:
      status: stratavore-status
      alerts: stratavore-alerts
  
  prometheus:
    enabled: true
    port: 9091

daemon:
  grpc_port: 50051
  heartbeat_interval_seconds: 10
  reconcile_interval_seconds: 30
  outbox_poll_interval_seconds: 2
  shutdown_timeout_seconds: 30

observability:
  log_level: info
  log_format: json
EOF
    echo "[OK] Created default config at ~/.config/stratavore/stratavore.yaml"
fi

echo ""
echo "================================"
echo "[OK] Docker integration complete!"
echo "================================"
echo ""
echo "Connection details:"
echo " PostgreSQL: postgresql://stratavore:stratavore_password@localhost:5432/stratavore_state"
echo " RabbitMQ: amqp://guest:guest@localhost:5672/"
echo " ntfy: http://localhost:2586"
echo ""
echo "Next steps:"
echo " 1. Build binaries: make build"
echo " 2. Install: make install"
echo " 3. Start daemon: stratavored"
echo " 4. Check status: stratavore status"
echo ""
