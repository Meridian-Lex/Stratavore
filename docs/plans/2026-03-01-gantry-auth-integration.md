# Stratavore Gantry Auth Integration Implementation Plan

> **For Lex:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix Stratavore's broken database authentication by integrating it properly into the Gantry postgres backend, with all credentials sourced from secrets.yaml.

**Architecture:** Gantry owns the postgres infrastructure (running as `stratavore-postgres` container, managed by `Gantry/storage/docker-compose.yml`). Stratavore gets its own `stratavore_state` database and `stratavore` user in that postgres. All credentials live in `/home/meridian/.config/secrets.yaml` — the sync script, docker-compose, and any other consumer reads from there. No hardcoded credentials anywhere.

**Tech Stack:** Bash, Python 3 (yaml reading), PostgreSQL 16 (pgvector), Docker Compose, Go (stratavore-migrate binary at `~/.local/bin/stratavore-migrate`)

---

### Task 1: Generate password and add to secrets.yaml

**Files:**
- Modify: `/home/meridian/.config/secrets.yaml`

**Step 1: Generate a strong password**

```bash
python3 -c "import secrets, string; print(''.join(secrets.choice(string.ascii_letters + string.digits) for _ in range(32)))"
```

Copy the output — this is `STRATAVORE_DB_PASSWORD`.

**Step 2: Add to secrets.yaml under the postgres key**

Open `/home/meridian/.config/secrets.yaml`. Under the `postgres:` key, add:

```yaml
    stratavore_db_password: <generated-password>
```

It should sit alongside `lex_db_password`, `postgres_password`, `synapse_db_password`.

**Step 3: Verify read**

```bash
python3 -c "import yaml; d=yaml.safe_load(open('/home/meridian/.config/secrets.yaml')); print(d['postgres']['stratavore_db_password'])"
```

Expected: prints the password, no KeyError.

**Step 4: Commit (secrets.yaml is NOT committed — just verify it's in .gitignore)**

```bash
git -C ~/meridian-home/lex-internal status
```

No action needed — secrets.yaml is a system file, not tracked in any repo.

---

### Task 2: Add STRATAVORE_DB_PASSWORD to Gantry docker-secrets.env

**Files:**
- Modify: `/home/meridian/meridian-home/projects/Gantry/docker-secrets.env`

**Step 1: Read the password**

```bash
STRAT_PW=$(python3 -c "import yaml; print(yaml.safe_load(open('/home/meridian/.config/secrets.yaml'))['postgres']['stratavore_db_password'])")
if grep -q '^STRATAVORE_DB_PASSWORD=' ~/meridian-home/projects/Gantry/docker-secrets.env; then
  sed -i.bak "s|^STRATAVORE_DB_PASSWORD=.*|STRATAVORE_DB_PASSWORD=${STRAT_PW}|" ~/meridian-home/projects/Gantry/docker-secrets.env
else
  printf 'STRATAVORE_DB_PASSWORD=%s\n' "$STRAT_PW" >> ~/meridian-home/projects/Gantry/docker-secrets.env
fi
```

**Step 2: Verify**

```bash
grep STRATAVORE_DB_PASSWORD ~/meridian-home/projects/Gantry/docker-secrets.env
```

Expected: `STRATAVORE_DB_PASSWORD=<password>`

**Step 3: Commit to Gantry**

```bash
cd ~/meridian-home/projects/Gantry
git checkout -b fix/stratavore-db-secrets
git add docker-secrets.env
git commit -m "fix: add STRATAVORE_DB_PASSWORD to docker-secrets for Stratavore integration"
```

---

### Task 3: Create stratavore user and database in live Gantry postgres

**Context:** The postgres container is already running with an existing data volume. Init scripts do not re-run. We must create the user and database manually against the live container.

**Step 1: Read the password into shell**

```bash
STRAT_PW=$(python3 -c "import yaml; print(yaml.safe_load(open('/home/meridian/.config/secrets.yaml'))['postgres']['stratavore_db_password'])")
```

**Step 2: Create user and database**

```bash
docker exec stratavore-postgres psql -U postgres -c "CREATE USER stratavore WITH PASSWORD '${STRAT_PW}';"
docker exec stratavore-postgres psql -U postgres -c "CREATE DATABASE stratavore_state OWNER stratavore;"
docker exec stratavore-postgres psql -U postgres -c "\c stratavore_state" -c "CREATE EXTENSION IF NOT EXISTS vector;"
docker exec stratavore-postgres psql -U postgres -c "GRANT ALL PRIVILEGES ON DATABASE stratavore_state TO stratavore;"
```

**Step 3: Verify**

```bash
docker exec stratavore-postgres psql -U postgres -c "\du" | grep stratavore
docker exec stratavore-postgres psql -U postgres -c "\l" | grep stratavore_state
```

Expected: `stratavore` user listed, `stratavore_state` database listed.

**Step 4: Test connection as stratavore user**

```bash
docker exec stratavore-postgres psql -U stratavore -d stratavore_state -c "SELECT 1;" 2>&1
```

Expected: ` ?column? ---------- 1`

---

### Task 4: Update Gantry init-db.sh for durable setup

**Files:**
- Modify: `/home/meridian/meridian-home/projects/Gantry/storage/postgres/init-db.sh`

**Step 1: Read current file**

```bash
cat ~/meridian-home/projects/Gantry/storage/postgres/init-db.sh
```

**Step 2: Add stratavore block**

After the synapse block, append:

```bash
    -- Stratavore orchestration database
    CREATE DATABASE stratavore_state;
    CREATE USER stratavore WITH PASSWORD '${STRATAVORE_DB_PASSWORD}';
    GRANT ALL PRIVILEGES ON DATABASE stratavore_state TO stratavore;
    \c stratavore_state
    CREATE EXTENSION IF NOT EXISTS vector;
    GRANT ALL ON SCHEMA public TO stratavore;
```

The `STRATAVORE_DB_PASSWORD` env var will be available because it's in `docker-secrets.env` (Task 2).

**Step 3: Commit**

```bash
cd ~/meridian-home/projects/Gantry
git add storage/postgres/init-db.sh
git commit -m "fix: add stratavore_state database and user to Gantry postgres init"
```

---

### Task 5: Run Stratavore migrations against stratavore_state

**Context:** `stratavore-migrate` binary is at `~/.local/bin/stratavore-migrate`. It reads `STRATAVORE_DB_URL` from environment.

**Step 1: Set DB URL from secrets**

```bash
STRAT_PW=$(python3 -c "import yaml; print(yaml.safe_load(open('/home/meridian/.config/secrets.yaml'))['postgres']['stratavore_db_password'])")
export STRATAVORE_DB_URL="postgresql://stratavore:${STRAT_PW}@localhost:5432/stratavore_state"
```

**Step 2: Run migrations (up)**

```bash
cd ~/meridian-home/projects/Stratavore
~/.local/bin/stratavore-migrate migrate --db-url "$STRATAVORE_DB_URL"
```

If the migrate subcommand differs, check:

```bash
~/.local/bin/stratavore-migrate --help
```

**Step 3: Verify schema created**

```bash
docker exec stratavore-postgres psql -U stratavore -d stratavore_state -c "\dt" 2>&1
```

Expected: 19 tables listed (projects, sessions, rank_tracking, directives, etc.)

**Step 4: Run V2 data import**

```bash
~/.local/bin/stratavore-migrate sync --db-url "$STRATAVORE_DB_URL" --v2-dir ~/meridian-home/lex-internal/state --type all
```

Expected: projects, rank events, directives imported. Sessions skip is acceptable (schema mismatch documented in MIGRATION-STATUS.md).

---

### Task 6: Fix the sync script to read from secrets.yaml

**Files:**
- Modify: `/home/meridian/stratavore-sync.sh`

**Step 1: Read current script**

```bash
cat ~/stratavore-sync.sh
```

**Step 2: Replace the hardcoded DB_URL line**

Replace:
```bash
DB_URL="postgresql://stratavore:<old-placeholder-replaced>@localhost:5432/stratavore_state"
```

With:
```bash
DB_PASSWORD=$(python3 -c "import yaml; print(yaml.safe_load(open('/home/meridian/.config/secrets.yaml'))['postgres']['stratavore_db_password'])")
DB_URL="postgresql://stratavore:${DB_PASSWORD}@localhost:5432/stratavore_state"
```

**Step 3: Run manually to verify**

```bash
bash ~/stratavore-sync.sh
```

Watch for: no auth errors, each sync step logs success.

**Step 4: Check log**

```bash
tail -20 ~/meridian-home/logs/stratavore-sync.log
```

Expected: all three sync sections (projects, config, rank) show success.

---

### Task 7: Update Stratavore docker-compose.yml

**Files:**
- Modify: `/home/meridian/meridian-home/projects/Stratavore/docker-compose.yml`

**Step 1: Remove the standalone postgres service block**

Find and remove the entire `postgres:` service stanza (image, container_name, environment, ports, volumes, healthcheck, networks). Gantry manages postgres.

Also remove `postgres_data` from the `volumes:` top-level block.

**Step 2: Update STRATAVORE_DATABASE_POSTGRESQL_* env vars**

On any service that references `STRATAVORE_DATABASE_POSTGRESQL_PASSWORD: <old-placeholder-replaced>`, update to read from the env file or set to a reference. The cleanest approach: add an `env_file` pointing to a generated secrets env or use `${STRATAVORE_DB_PASSWORD}` which will be in the environment when docker-compose runs (sourced from Gantry's docker-secrets.env or a Stratavore-specific env file).

Update references:
- `STRATAVORE_DATABASE_POSTGRESQL_HOST: postgres` → `STRATAVORE_DATABASE_POSTGRESQL_HOST: localhost`
- `STRATAVORE_DATABASE_POSTGRESQL_PASSWORD: <old-placeholder-replaced>` → `STRATAVORE_DATABASE_POSTGRESQL_PASSWORD: ${STRATAVORE_DB_PASSWORD}`

**Step 3: Audit for any other hardcoded credential strings**

```bash
grep -r "<old-placeholder-replaced>" ~/meridian-home/projects/Stratavore/ --include="*.yml" --include="*.yaml" --include="*.sh" --include="*.go" --include="*.env"
```

Expected: zero matches after edit.

**Step 4: Commit**

```bash
cd ~/meridian-home/projects/Stratavore
git checkout -b fix/gantry-auth-integration
git add docker-compose.yml
git commit -m "fix: remove standalone postgres, use Gantry backend with secrets-sourced credentials"
```

---

### Task 8: Audit and clean remaining hardcoded credentials

**Step 1: Full credential audit across Stratavore tree**

```bash
grep -rn "<old-placeholder-replaced>\|stratavore_password" \
  ~/meridian-home/projects/Stratavore/ \
  --include="*.go" --include="*.sh" --include="*.yml" \
  --include="*.yaml" --include="*.md" --include="*.env" \
  --include="*.toml" --include="*.json"
```

**Step 2: Fix any remaining hits**

For each match: replace hardcoded value with a secrets.yaml read or environment variable reference as appropriate to context.

**Step 3: Commit any fixes**

```bash
git add -p
git commit -m "fix: remove remaining hardcoded stratavore DB credentials"
```

---

### Task 9: Verify end-to-end and update status docs

**Step 1: Trigger manual sync run**

```bash
bash ~/stratavore-sync.sh
tail -30 ~/meridian-home/logs/stratavore-sync.log
```

Expected: all sections clean, no auth errors.

**Step 2: Verify cron is still registered**

```bash
crontab -l | grep stratavore
```

Expected: `*/5 * * * * /home/meridian/stratavore-sync.sh >> ...`

**Step 3: Verify containers healthy**

```bash
docker ps --format "table {{.Names}}\t{{.Status}}" | grep stratavore
```

Expected: all containers showing healthy/running.

**Step 4: Update SESSION-STATE.md**

```bash
cat ~/meridian-home/projects/Stratavore/SESSION-STATE.md
```

Update to reflect: auth fixed, database stratavore_state live in Gantry postgres, sync operational.

**Step 5: Commit status docs and push**

```bash
cd ~/meridian-home/projects/Stratavore
git add SESSION-STATE.md
git commit -m "docs: update session state — auth fixed, Gantry integration complete"
git push origin fix/gantry-auth-integration
```

**Step 6: Open PR**

```bash
gh pr create \
  --repo Meridian-Lex/Stratavore \
  --head Meridian-Lex:fix/gantry-auth-integration \
  --assignee LunarLaurus \
  --title "fix: integrate Stratavore with Gantry postgres backend, secrets.yaml auth" \
  --body "$(cat <<'EOF'
## Summary

- Removes standalone postgres service from Stratavore docker-compose (Gantry owns the data plane)
- Creates stratavore_state database and stratavore user in Gantry postgres
- All credentials now sourced from secrets.yaml — zero hardcoded values
- Fixes broken cron sync (was failing SASL auth since stratavore user did not exist)
- Updates Gantry init-db.sh for durable setup on future container restarts

## Test plan

- [ ] stratavore-sync.sh runs cleanly with no auth errors
- [ ] All 22 schema tables present in stratavore_state
- [ ] V2 data re-imported (projects, rank events, directives)
- [ ] grep for <old-placeholder-replaced> returns zero matches
- [ ] Cron sync log clean for 15+ minutes

EOF
)"
```

---

### Task 10: Add secrets wrapper to task queue

**Files:**
- Modify: `/home/meridian/meridian-home/lex-internal/state/TASK-QUEUE.md`

Add a QUEUED task for building a small secrets accessor utility (Go binary or Python CLI) so all fleet scripts can read secrets.yaml cleanly without inline python3 one-liners.

Commit and push to lex-state.

---

## Autonomous Scope (runs while Admiral tests)

Tasks 1–9 are fully autonomous — no user interaction required. Task 10 (queue entry) is a state commit to lex-internal.

The Admiral's user-testing of Stratavore commands (`stratavore status`, `stratavore projects`, session launch) is independent of this work and can proceed in parallel once Task 5 (migrations) completes and the database is live.
