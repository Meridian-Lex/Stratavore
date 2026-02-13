# Stratavore - AI Development Workspace Orchestrator

**Layer-devouring intelligence; evokes an omnipresent orchestrator.**

Stratavore is a comprehensive workspace orchestration system for AI-assisted development. It manages Meridian Lex sessions as first-class resources, tracks global state, enables multi-project parallel workflows, and integrates with Docker infrastructure for persistence, messaging, and observability.

## Features

- **Multi-Runner Management** - Run multiple Meridian Lex sessions simultaneously across different projects
- IN PROGRESS **Session Resumption** - Resume work instantly from anywhere with full context preservation
- **Global Visibility** - Always know what's running where with comprehensive dashboards
- **State Persistence** - PostgreSQL + pgvector for reliable state and session tracking
- **Event-Driven** - RabbitMQ for real-time event distribution and coordination
- **Observability** - Prometheus metrics, Grafana dashboards, structured logging
- **Resource Management** - Track tokens, manage quotas, prevent overruns
- **Transactional Outbox** - Guaranteed event delivery with no message loss

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│ STRATAVORE CONTROL PLANE │
│ (stratavored) │
│ │
│ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ │
│ │ State Manager│ │ Runner Mgr │ │ Event Bus │ │
│ │ (PostgreSQL) │ │ (Lifecycle) │ │ (RabbitMQ) │ │
│ └──────────────┘ └──────────────┘ └──────────────┘ │
└─────────────────────────────────────────────────────────┘
                         │
        ┌────────────────┼────────────────┐
        │ │ │
   ┌────▼────┐ ┌────▼────┐ ┌────▼────┐
   │ Agent │ │ Agent │ │ Agent │
   │ Wrapper │ │ Wrapper │ │ Wrapper │
   └────┬────┘ └────┬────┘ └────┬────┘
        │ │ │
   ┌────▼────┐ ┌────▼────┐ ┌────▼────┐
│ Lex │ │ Lex │ │ Lex │
   │ Code │ │ Code │ │ Code │
   └─────────┘ └─────────┘ └─────────┘
```

## Lex WebUI

The browser-based control plane for Stratavore is maintained as a git submodule at `stratavore-ui/`.

```bash
# Clone with submodule
git clone --recurse-submodules git@github.com:Meridian-Lex/Stratavore.git

# If already cloned without submodule
git submodule update --init --recursive
```

The WebUI connects to the Stratavore HTTP API and gRPC daemon. See [`stratavore-ui/README.md`](stratavore-ui/README.md) for frontend setup, environment configuration, and deployment instructions.

---

## Quick Start

### Prerequisites

- Go 1.22 or later
- PostgreSQL 14+ with pgvector extension
- RabbitMQ 3.12+
- Docker (for Gantry integration)

### Installation

```bash
# Clone repository (include submodules for Lex WebUI)
git clone --recurse-submodules git@github.com:Meridian-Lex/Stratavore.git
cd Stratavore

# Build binaries
make build

# Setup Docker integration (if using Gantry)
./scripts/setup-docker-integration.sh

# Install binaries
sudo make install

# Install systemd service (optional)
sudo make systemd-install
```

### Configuration

Configuration file locations (in order of precedence):
1. `--config` flag
2. `~/.config/stratavore/stratavore.yaml`
3. `/etc/stratavore/stratavore.yaml`
4. Environment variables with `STRATAVORE_` prefix

Example configuration:

```yaml
database:
  postgresql:
    host: localhost
    port: 5432
    database: stratavore_state
    user: stratavore
    password: your_password

docker:
  rabbitmq:
    host: localhost
    port: 5672
    exchange: stratavore.events
    publisher_confirms: true
  
  prometheus:
    enabled: true
    port: 9091

daemon:
  grpc_port: 50051
  heartbeat_interval_seconds: 10
  reconcile_interval_seconds: 30
```

### Usage

```bash
# Start the daemon
stratavored

# Or as systemd service
sudo systemctl start stratavored

# Launch a project (smart launch - attaches if exists, creates if not)
stratavore myproject

# Launch with god mode
stratavore myproject --god

# Global status dashboard
stratavore status

# List all active runners
stratavore runners

# List all projects
stratavore projects

# Create new project
stratavore new my-new-project

# Attach to specific runner
stratavore attach runner_abc123

# Stop a runner
stratavore kill runner_abc123
```

## Database Schema

Stratavore uses PostgreSQL with the following key tables:

- **projects** - Project metadata and statistics
- **runners** - Active and historical Meridian Lex instances
- **sessions** - Conversation history and resumption data
- **outbox** - Transactional outbox for reliable event delivery
- **events** - Immutable audit log for event sourcing
- **token_budgets** - Token usage tracking and limits
- **resource_quotas** - Per-project resource constraints

See `migrations/postgres/` for full schema.

## Event System

Stratavore uses RabbitMQ for event-driven architecture with the following routing keys:

- `runner.started.<project>` - Runner launched
- `runner.stopped.<project>` - Runner terminated
- `runner.failed.<project>` - Runner crashed
- `runner.heartbeat.<runner_id>` - Health updates
- `session.created.<project>` - New session started
- `system.alert.<severity>` - System alerts

## Metrics

Prometheus metrics are exposed on port 9091 (configurable):

- `stratavore_runners_total{status="running|paused|terminated"}`
- `stratavore_runners_by_project{project="name"}`
- `stratavore_sessions_total`
- `stratavore_tokens_used_total{scope="global|project|runner"}`
- `stratavore_heartbeat_latency_seconds` (histogram)

## Documentation

For comprehensive documentation, see the [docs/](docs/) directory:

### [User Documentation](docs/user/)
- [Quick Start Guide](docs/user/quick-start.md) - Get up and running in 5 minutes
- [User Guide](docs/user/guide.md) - Complete user manual
- [CLI Reference](docs/user/cli.md) - All commands and options
- [FAQ](docs/user/faq.md) - Frequently asked questions

### [Developer Documentation](docs/developer/)
- [Architecture](docs/developer/architecture.md) - System design and patterns
- [Development Guide](docs/developer/development.md) - Setup and workflow
- [Testing](docs/developer/testing.md) - Testing strategies
- [Contributing](docs/developer/contributing.md) - How to contribute

### [Operations Documentation](docs/operations/)
- [Deployment Guide](docs/operations/deployment.md) - Production deployment
- [Monitoring](docs/operations/) - Observability and troubleshooting
- [Windows Support](docs/operations/windows.md) - Windows-specific instructions

### [API Documentation](docs/api/)
- [gRPC API](docs/api/grpc.md) - Complete API reference
- [Protocol Buffers](docs/api/protobuf.md) - Schema documentation

## Development

```bash
# Run tests
make test

# Run integration tests (requires Docker)
make test-integration

# Run linters
make lint

# Format code
make format

# Run daemon in dev mode
make run-daemon

# Run CLI in dev mode
make run-cli
```

## Architecture Decisions

### Transactional Outbox Pattern

Stratavore uses the transactional outbox pattern to guarantee event delivery:

1. Runner creation and event insertion happen in same DB transaction
2. Background publisher polls outbox table
3. Events published to RabbitMQ with publisher confirms
4. Delivered events marked in database
5. Failed deliveries retry with exponential backoff

This ensures zero message loss even if RabbitMQ is temporarily unavailable.

### Advisory Locks for Quotas

Resource quota checks use PostgreSQL advisory locks to prevent race conditions:

```sql
SELECT pg_advisory_xact_lock(hash_project($1));
-- Check current runner count
-- Insert new runner if under quota
```

This guarantees atomic quota enforcement across concurrent launches.

### Heartbeat TTL and Reconciliation

Stale runners are detected via:
- Agents send heartbeats every 10 seconds
- Daemon reconciliation loop runs every 30 seconds
- Runners with `last_heartbeat` older than TTL are marked failed
- Automatic cleanup prevents orphaned state

## Security Considerations

- **Authentication**: gRPC with optional mTLS (configure in `security` section)
- **Join Tokens**: Ephemeral tokens for agent registration with configurable TTL
- **Database Credentials**: Store in environment variables or secret files
- **Audit Logging**: All actions logged to `events` table with HMAC signatures

## Production Deployment

### High Availability

For HA deployment:
1. Run multiple daemon instances (they coordinate via database)
2. Use PostgreSQL with replication
3. RabbitMQ cluster with mirrored queues
4. Load balance CLI requests via gRPC

### Monitoring

Recommended monitoring setup:
- Prometheus scraping daemon metrics
- Grafana dashboards for visualization
- Loki for log aggregation
- Alertmanager for critical alerts via ntfy

### Backup Strategy

Critical data to backup:
- PostgreSQL database (pg_dump or WAL archiving)
- Session transcripts (S3 or object storage)
- Configuration files

## Troubleshooting

### Daemon won't start

```bash
# Check logs
journalctl -u stratavored -f

# Verify database connection
psql -h localhost -U stratavore -d stratavore_state

# Check RabbitMQ
docker ps | grep rabbitmq
```

### Stale runners not cleaned up

```bash
# Manual reconciliation
stratavore reconcile

# Check reconciliation settings
grep reconcile_interval ~/.config/stratavore/stratavore.yaml
```

### Events not being published

```bash
# Check outbox table
psql stratavore_state -c "SELECT count(*) FROM outbox WHERE delivered = false;"

# Check RabbitMQ connection
docker logs lex-rabbitmq
```

## Contributing

Contributions welcome! Please:
1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure `make lint` passes
5. Submit a pull request

## License

MIT License - see LICENSE file for details

## Acknowledgments

- Built for managing Meridian Lex ()
- Inspired by Kubernetes and process supervisors
- Integrates with Gantry infrastructure

## Roadmap

- [x] Web UI for dashboard (stratavore-ui submodule — see `stratavore-ui/`)
- [ ] Remote runners (multi-node support)
- [ ] Session similarity search via Qdrant
- [ ] Auto-scaling based on load
- [ ] Workflow automation
- [ ] Team collaboration features
- [ ] Advanced scheduling policies

---

**Stratavore**: Orchestrate AI development at scale.
