# Database Conventions

## Migrations

Files: `migrations/postgres/NNNN_name.up.sql` and `NNNN_name.down.sql`

- Four-digit zero-padded sequence: `0001_initial.up.sql`
- Every up migration has a matching down migration
- Extensions in migration `0000_extensions`

## Schema Defaults

```sql
id UUID PRIMARY KEY DEFAULT gen_random_uuid()
created_at TIMESTAMPTZ DEFAULT NOW()
updated_at TIMESTAMPTZ DEFAULT NOW()
```

- Always `TIMESTAMPTZ` — never `TIMESTAMP` (timezone-aware)
- UUIDs for all primary keys via `gen_random_uuid()`
- `updated_at` on all mutable tables

## JSONB for Flexible Fields

Use `JSONB` for lists and maps that vary per record. Not nullable arrays.

```sql
flags        JSONB DEFAULT '[]'
capabilities JSONB DEFAULT '[]'
environment  JSONB DEFAULT '{}'
backend_config JSONB DEFAULT '{}'
```

## Enum Types

Define PostgreSQL enum types before tables that use them.

```sql
CREATE TYPE runner_status AS ENUM ('starting', 'running', 'paused', 'terminated', 'failed');
```

## Connection Pool

pgxpool with explicit `MaxConns`, `MinConns`, `MaxConnLifetime` (1h), `MaxConnIdleTime` (30m).
Always `Ping` after pool creation to verify connection.

## Data-Driven Config

Prefer DB tables over hardcoded enums for extensible config (e.g. `model_registry`).
New models added via DB row — no code change required.
