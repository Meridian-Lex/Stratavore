# Stratavore Quick Start Guide

Get up and running with Stratavore in 5 minutes.

## Prerequisites Check

```bash
# Check Go version (need 1.22+)
go version

# Check Docker is running
docker ps

# Check PostgreSQL is accessible (or will be via Docker)
psql --version
```

## Step 1: Build Stratavore

```bash
cd stratavore

# Download dependencies
make deps

# Build all components
make build

# Verify binaries created
ls -lh bin/
# Should see: stratavore, stratavored, stratavore-agent
```

## Step 2: Setup Infrastructure

### Option A: Using Gantry (Recommended)

If you have Gantry infrastructure running:

```bash
# This script will:
# - Create PostgreSQL database and user
# - Run database migrations
# - Configure RabbitMQ exchanges and queues
# - Setup ntfy topics
# - Create default configuration
./scripts/setup-docker-integration.sh
```

### Option B: Manual Setup

1. **PostgreSQL**:
```bash
# Create database and user
createdb stratavore_state
psql stratavore_state < migrations/postgres/0000_extensions.up.sql
psql stratavore_state < migrations/postgres/0001_initial.up.sql
```

2. **RabbitMQ**:
```bash
# Start RabbitMQ (if not running)
docker run -d --name rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3-management

# Access management UI: http://localhost:15672 (guest/guest)
# Create exchange: stratavore.events (type: topic)
```

3. **Configuration**:
```bash
mkdir -p ~/.config/stratavore
cp configs/stratavore.yaml ~/.config/stratavore/

# Edit with your credentials
nano ~/.config/stratavore/stratavore.yaml
```

## Step 3: Install Binaries

```bash
# Install to /usr/local/bin
sudo make install

# Verify installation
which stratavore
which stratavored
which stratavore-agent

# Check version
stratavore --version
```

## Step 4: Start the Daemon

### Option A: Run in foreground (development)

```bash
# Terminal 1: Start daemon
stratavored

# You should see:
# {"level":"info","ts":"...","msg":"starting stratavore daemon"}
# {"level":"info","ts":"...","msg":"connected to postgresql"}
# {"level":"info","ts":"...","msg":"connected to rabbitmq"}
# {"level":"info","ts":"...","msg":"stratavore daemon started successfully"}
```

### Option B: Run as systemd service (production)

```bash
# Install service
sudo make systemd-install

# Start service
sudo systemctl start stratavored

# Enable on boot
sudo systemctl enable stratavored

# Check status
sudo systemctl status stratavored

# View logs
journalctl -u stratavored -f
```

## Step 5: Create Your First Project

```bash
# Create a new project
cd ~/my-awesome-project
stratavore new my-awesome-project

# Verify it was created
stratavore projects
# Output:
# === Projects ===
# my-awesome-project (idle) - 0 active runners
```

## Step 6: Launch a Runner

```bash
# Launch Meridian Lex for the project
stratavore my-awesome-project

# The system will:
# 1. Check for existing runners
# 2. Create new runner if none exist
# 3. Start stratavore-agent wrapper
# 4. Launch Meridian Lex
# 5. Begin heartbeat monitoring

# In another terminal, check status
stratavore status
# Output:
# === Stratavore Status ===
# Active Runners: 1
# Active Projects: 1
# Total Sessions: 1
```

## Step 7: Explore Commands

```bash
# List all active runners
stratavore runners

# Global status dashboard
stratavore status

# List projects
stratavore projects

# Attach to a runner (if you need to reconnect)
stratavore attach <runner-id>

# Stop a runner
stratavore kill <runner-id>

# Check daemon status
stratavore daemon status
```

## Common Workflows

### Launch Multiple Runners for Same Project

```bash
# Terminal 1
stratavore my-project

# Terminal 2
stratavore my-project
# Will prompt: Attach to existing or launch new?
```

### Work on Multiple Projects Simultaneously

```bash
# Terminal 1: Project A
stratavore project-a

# Terminal 2: Project B
stratavore project-b

# Terminal 3: Project C
stratavore project-c

# Monitor all from Terminal 4
watch -n 2 stratavore status
```

### Resume a Session

```bash
# Sessions are automatically tracked
# If you disconnect and reconnect:
stratavore my-project
# Will automatically attach to existing runner if still active
```

## Monitoring

### View Metrics

```bash
# Prometheus metrics
curl http://localhost:9091/metrics

# Key metrics:
# - stratavore_runners_total
# - stratavore_tokens_used_total
# - stratavore_heartbeat_latency_seconds
```

### Check Logs

```bash
# Daemon logs (if systemd)
journalctl -u stratavored -f

# Daemon logs (if foreground)
# Already visible in terminal

# Database events
psql stratavore_state -c "SELECT * FROM events ORDER BY timestamp DESC LIMIT 10;"
```

### Subscribe to Notifications

```bash
# In a terminal, subscribe to ntfy
curl -s http://localhost:2586/stratavore-status/sse

# Or use ntfy CLI
ntfy subscribe stratavore-status

# You'll receive notifications for:
# - Runners started/stopped
# - System alerts
# - Token budget warnings
```

## Troubleshooting

### "Failed to connect to database"

```bash
# Check PostgreSQL is running
pg_isready -h localhost

# Test connection
psql -h localhost -U stratavore -d stratavore_state

# Check config
cat ~/.config/stratavore/stratavore.yaml | grep postgresql
```

### "Failed to connect to rabbitmq"

```bash
# Check RabbitMQ is running
docker ps | grep rabbitmq

# Test connection
curl http://localhost:15672/api/overview
```

### "Runner not responding"

```bash
# Check active runners
stratavore runners

# Manual reconciliation
# (daemon does this automatically every 30s)
psql stratavore_state -c "SELECT reconcile_stale_runners(30);"
```

### "Command not found: stratavore"

```bash
# Verify installation
which stratavore

# If not found, add to PATH or reinstall
sudo make install

# Or run directly
./bin/stratavore
```

## Next Steps

- **Configure Resource Quotas**: Set limits per project
- **Setup Grafana**: Visualize metrics
- **Enable mTLS**: Secure gRPC communication
- **Explore Web UI**: (Future feature)
- **Setup Backups**: Backup PostgreSQL database

## Getting Help

- **Documentation**: See README.md
- **Architecture**: See ARCHITECTURE.md
- **GitHub Issues**: Report bugs or request features
- **Logs**: Check `journalctl -u stratavored -f`

---

**You're now ready to orchestrate AI development at scale with Stratavore!** 
