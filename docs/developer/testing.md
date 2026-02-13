# Stratavore Testing & Validation Guide

This guide walks you through testing Stratavore components to verify everything works correctly.

## Prerequisites

```bash
# Ensure you have:
- PostgreSQL 14+ with pgvector
- RabbitMQ 3.12+
- Go 1.22+
- protoc compiler
- Docker (for Gantry integration)
```

## Setup for Testing

### 1. Install protobuf compiler

```bash
# Ubuntu/Debian
sudo apt install -y protobuf-compiler

# macOS
brew install protobuf

# Install Go plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Add to PATH if needed
export PATH="$PATH:$(go env GOPATH)/bin"
```

### 2. Setup infrastructure

```bash
cd stratavore

# If using Gantry
./scripts/setup-docker-integration.sh

# Or manually setup PostgreSQL and RabbitMQ
# See QUICKSTART.md for manual setup
```

### 3. Generate protobuf code

```bash
make proto

# This creates:
# - pkg/api/stratavore.pb.go
# - pkg/api/stratavore_grpc.pb.go
```

### 4. Build binaries

```bash
make build

# Verify binaries
ls -lh bin/
# Should see: stratavore, stratavored, stratavore-agent
```

## Component Testing

### Test 1: Database Connectivity

```bash
# Test PostgreSQL connection
psql -h localhost -U stratavore -d stratavore_state -c "SELECT version();"

# Verify tables exist
psql -h localhost -U stratavore -d stratavore_state -c "\dt"

# Expected tables:
# - projects
# - runners
# - sessions
# - outbox
# - events
# - resource_quotas
# - token_budgets
# - daemon_state
# - agent_tokens

# Test transactional outbox function
psql -h localhost -U stratavore -d stratavore_state -c "SELECT reconcile_stale_runners(30);"
```

### Test 2: RabbitMQ Connectivity

```bash
# Check RabbitMQ is running
docker ps | grep rabbitmq

# Access management UI
# http://localhost:15672 (guest/guest)

# Verify exchange exists
curl -u guest:guest http://localhost:15672/api/exchanges/%2F/stratavore.events

# Should see exchange with type=topic, durable=true
```

### Test 3: Daemon Startup

```bash
# Terminal 1: Start daemon with debug logging
STRATAVORE_OBSERVABILITY_LOG_LEVEL=debug ./bin/stratavored

# Expected output:
# {"level":"info","msg":"starting stratavore daemon","version":"..."}
# {"level":"info","msg":"connecting to postgresql"}
# {"level":"info","msg":"connected to postgresql"}
# {"level":"info","msg":"connecting to rabbitmq"}
# {"level":"info","msg":"connected to rabbitmq"}
# {"level":"info","msg":"outbox publisher started"}
# {"level":"info","msg":"reconciliation loop started"}
# {"level":"info","msg":"metrics server starting","port":9091}
# {"level":"info","msg":"gRPC server starting","port":50051}
# {"level":"info","msg":"stratavore daemon started successfully"}
```

### Test 4: Metrics Endpoint

```bash
# Check metrics are exposed
curl http://localhost:9091/metrics

# Expected metrics:
# stratavore_runners_total{status="..."}
# stratavore_daemon_uptime_seconds
# stratavore_sessions_total
# stratavore_tokens_used_total

# Health check
curl http://localhost:9091/health
# Should return: OK
```

### Test 5: Notifications (ntfy)

```bash
# Terminal 2: Subscribe to notifications
curl -s http://localhost:2586/stratavore-status/sse

# You should see startup notification:
# {"topic":"stratavore-status","title":"Stratavore Daemon Started",...}

# Try manual notification
./scripts/test-notification.sh  # If exists
```

### Test 6: Database Operations

```bash
# Create a test project
psql -h localhost -U stratavore -d stratavore_state << EOF
INSERT INTO projects (name, path, status) 
VALUES ('test-project', '/tmp/test', 'active');
EOF

# Verify project created
psql -h localhost -U stratavore -d stratavore_state -c "SELECT * FROM projects WHERE name='test-project';"

# Test CLI project listing
./bin/stratavore projects

# Should show test-project
```

### Test 7: Outbox Publisher

```bash
# Insert test event into outbox
psql -h localhost -U stratavore -d stratavore_state << EOF
INSERT INTO outbox (event_type, payload, routing_key)
VALUES ('test.event', '{"message": "test"}', 'test.routing.key');
EOF

# Check outbox table
psql -h localhost -U stratavore -d stratavore_state -c "SELECT * FROM outbox WHERE delivered = false;"

# Wait 5 seconds for publisher to process
sleep 5

# Verify event delivered
psql -h localhost -U stratavore -d stratavore_state -c "SELECT delivered, delivered_at FROM outbox WHERE event_type='test.event';"

# Should show delivered=true
```

### Test 8: Reconciliation

```bash
# Insert a stale runner (simulating crashed runner)
psql -h localhost -U stratavore -d stratavore_state << EOF
INSERT INTO runners (runtime_type, runtime_id, project_name, project_path, status, last_heartbeat)
VALUES ('process', '99999', 'test-project', '/tmp/test', 'running', NOW() - INTERVAL '60 seconds');
EOF

# Wait 30 seconds for reconciliation
sleep 30

# Check runner status
psql -h localhost -U stratavore -d stratavore_state -c "SELECT status, terminated_at FROM runners WHERE runtime_id='99999';"

# Should show status='failed' with terminated_at set
```

### Test 9: gRPC API (after protobuf compilation)

```bash
# Test with grpcurl
grpcurl -plaintext localhost:50051 list

# Should show:
# stratavore.StratavoreService

grpcurl -plaintext localhost:50051 list stratavore.StratavoreService

# Should list all RPC methods

# Test GetStatus RPC
grpcurl -plaintext localhost:50051 stratavore.StratavoreService/GetStatus

# Should return daemon status and metrics
```

### Test 10: Session Management

```bash
# Create test session
psql -h localhost -U stratavore -d stratavore_state << EOF
INSERT INTO sessions (id, runner_id, project_name, resumable)
VALUES ('test-session-1', 'runner-id-123', 'test-project', true);
EOF

# Query resumable sessions
psql -h localhost -U stratavore -d stratavore_state -c "
SELECT id, project_name, resumable, started_at 
FROM sessions 
WHERE project_name='test-project' AND resumable=true;
"

# Should show test-session-1
```

## Integration Tests

### End-to-End Runner Lifecycle

```bash
# This would be the full workflow once CLI gRPC is implemented:

# 1. Create project
./bin/stratavore new e2e-test-project

# 2. Launch runner
./bin/stratavore e2e-test-project

# 3. Verify runner created
./bin/stratavore runners

# 4. Check metrics
curl http://localhost:9091/metrics | grep stratavore_runners_total

# 5. Check notifications
# Should receive "Runner Started" notification

# 6. Simulate heartbeat (manual for now)
# Agent would send these automatically

# 7. Stop runner
./bin/stratavore kill <runner-id>

# 8. Verify cleanup
./bin/stratavore runners
# Should show runner terminated
```

## Performance Testing

### Load Test: Outbox Publisher

```bash
# Insert 1000 events
psql -h localhost -U stratavore -d stratavore_state << EOF
INSERT INTO outbox (event_type, payload, routing_key)
SELECT 
  'load.test', 
  '{"index": ' || generate_series || '}',
  'load.test.key'
FROM generate_series(1, 1000);
EOF

# Monitor delivery
watch -n 1 "psql -h localhost -U stratavore -d stratavore_state -t -c \"
SELECT 
  COUNT(*) FILTER (WHERE delivered = false) as pending,
  COUNT(*) FILTER (WHERE delivered = true) as delivered,
  COUNT(*) as total
FROM outbox 
WHERE event_type = 'load.test';
\""

# Should see pending decrease to 0 within ~30 seconds
```

### Load Test: Concurrent Quota Checks

```bash
# This tests advisory lock performance
# Run multiple concurrent launches (simulated)

for i in {1..10}; do
  (
    psql -h localhost -U stratavore -d stratavore_state << EOF
    BEGIN;
    SELECT pg_advisory_xact_lock(hash_project('test-project'));
    SELECT count(*) FROM runners WHERE project_name='test-project';
    COMMIT;
EOF
  ) &
done

wait

# All should complete without deadlocks
```

## Monitoring Tests

### Prometheus Scraping

```bash
# Add to Prometheus config (if using Prometheus)
cat >> /etc/prometheus/prometheus.yml << EOF
  - job_name: 'stratavore'
    static_configs:
      - targets: ['localhost:9091']
    scrape_interval: 10s
EOF

# Reload Prometheus
curl -X POST http://localhost:9090/-/reload

# Query metrics
curl 'http://localhost:9090/api/v1/query?query=stratavore_daemon_uptime_seconds'
```

### Grafana Dashboard

```bash
# Import dashboard JSON (to be created)
# Dashboard should show:
# - Active runners over time
# - Runner status distribution
# - Tokens used
# - Heartbeat latency
# - Daemon uptime
```

## Troubleshooting Tests

### Test: Daemon won't start

```bash
# Check database connectivity
pg_isready -h localhost -p 5432

# Check RabbitMQ connectivity
curl http://localhost:15672/api/overview

# Check config file
cat ~/.config/stratavore/stratavore.yaml

# Check logs
journalctl -u stratavored -f --no-pager
```

### Test: Events not publishing

```bash
# Check outbox entries
psql -h localhost -U stratavore -d stratavore_state -c "
SELECT count(*), delivered 
FROM outbox 
GROUP BY delivered;
"

# Check RabbitMQ queues
curl -u guest:guest http://localhost:15672/api/queues/%2F/stratavore.daemon.events

# Check daemon logs for publisher errors
grep "outbox" /var/log/stratavore/*.log
```

### Test: Metrics not updating

```bash
# Check metrics server is running
curl -I http://localhost:9091/health

# Check metrics update loop
# Daemon should log periodic updates

# Verify runner manager has runners
curl http://localhost:9091/metrics | grep stratavore_runners_total
```

## Cleanup After Tests

```bash
# Stop daemon
killall stratavored

# Clean test data
psql -h localhost -U stratavore -d stratavore_state << EOF
DELETE FROM sessions WHERE project_name = 'test-project';
DELETE FROM runners WHERE project_name = 'test-project';
DELETE FROM projects WHERE name = 'test-project';
DELETE FROM outbox WHERE event_type = 'load.test';
EOF

# Or reset entire database
psql -h localhost -U stratavore -d stratavore_state -f migrations/postgres/0001_initial.down.sql
psql -h localhost -U stratavore -d stratavore_state -f migrations/postgres/0000_extensions.down.sql
./scripts/migrate.sh up
```

## Automated Test Suite (Future)

```bash
# Unit tests
make test

# Integration tests
make test-integration

# E2E tests
make test-e2e

# All tests with coverage
make test-all

# Benchmark tests
make bench
```

## Success Criteria

- [x] Database migrations run successfully
- [x] Daemon starts without errors
- [x] Metrics endpoint responds
- [x] Outbox publisher delivers events
- [x] Reconciliation cleans stale runners
- [x] Notifications are sent
- [x] Sessions tracked in database
- [ ] gRPC API responds (after proto compilation)
- [ ] CLI communicates with daemon
- [ ] End-to-end runner launch works
- [ ] Load test: 100+ concurrent operations
- [ ] Zero message loss under failure

## Reporting Issues

If tests fail:

1. Check logs: `journalctl -u stratavored -f`
2. Verify config: `cat ~/.config/stratavore/stratavore.yaml`
3. Test connectivity: PostgreSQL, RabbitMQ, ntfy
4. Check GitHub issues
5. Include logs, config, and error messages

---

**Testing Status**: All unit components testable, E2E pending gRPC compilation
