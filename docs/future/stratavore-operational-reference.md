# Stratavore + Claude Code: Operational Reference Implementation

## Purpose

This document provides ready-to-use configuration files, hook scripts, MCP server definitions, and skill templates that enable Stratavore agents to operate effectively in auto mode. Copy and adapt these for your projects.

---

## Part 1: Project Bootstrap Configuration

When creating a new project under Stratavore, initialize with this structure:

### Project Root Structure

```
myproject/
├── CLAUDE.md                           # Session-level context
├── .claude/
│   ├── settings.json                   # Hooks, MCP, skills config
│   ├── settings.local.json             # Local overrides (gitignored)
│   ├── hooks/
│   │   ├── quota-check.sh
│   │   ├── protect-critical.sh
│   │   ├── auto-checkpoint.sh
│   │   └── emit-events.sh
│   ├── skills/
│   │   ├── project-architecture.md
│   │   ├── api-design.md
│   │   ├── deployment-guide.md
│   │   └── data-model.md
│   └── agents/
│       ├── investigator.json
│       ├── verifier.json
│       └── integration-tester.json
├── docs/
│   ├── ARCHITECTURE.md
│   ├── API.md
│   ├── SCHEMA.md
│   └── DEPLOYMENT.md
└── [project files]
```

### Bootstrap Script

Create a script that Stratavore runs on new project creation:

```bash
#!/bin/bash
# scripts/init-stratavore-project.sh
# Run by Stratavore daemon when --init-claude-code flag is set

PROJECT_DIR="${1:-.}"
PROJECT_NAME=$(basename "$PROJECT_DIR")

echo "Initializing Stratavore+Claude Code environment for $PROJECT_NAME..."

# Create directory structure
mkdir -p "$PROJECT_DIR/.claude/hooks"
mkdir -p "$PROJECT_DIR/.claude/skills"
mkdir -p "$PROJECT_DIR/.claude/agents"

# Copy hook templates
cp templates/hooks/quota-check.sh "$PROJECT_DIR/.claude/hooks/"
cp templates/hooks/protect-critical.sh "$PROJECT_DIR/.claude/hooks/"
cp templates/hooks/auto-checkpoint.sh "$PROJECT_DIR/.claude/hooks/"
cp templates/hooks/emit-events.sh "$PROJECT_DIR/.claude/hooks/"
chmod +x "$PROJECT_DIR/.claude/hooks/"*.sh

# Copy skill templates
cp templates/skills/project-architecture.md "$PROJECT_DIR/.claude/skills/"
cp templates/skills/api-design.md "$PROJECT_DIR/.claude/skills/"
cp templates/skills/deployment-guide.md "$PROJECT_DIR/.claude/skills/"

# Copy agent definitions
cp templates/agents/investigator.json "$PROJECT_DIR/.claude/agents/"
cp templates/agents/verifier.json "$PROJECT_DIR/.claude/agents/"

# Generate project-specific CLAUDE.md
cat > "$PROJECT_DIR/CLAUDE.md" <<'CLAUDE_EOF'
# $PROJECT_NAME Development Context

## Session Identity

- **Project**: $PROJECT_NAME
- **Runner ID**: $STRATAVORE_RUNNER_ID
- **Session ID**: $STRATAVORE_SESSION_ID
- **Started**: $(date -Iseconds)

## Project Structure

\`\`\`
docs/          - API, architecture, schema documentation
src/           - Source code
tests/         - Test suite
.claude/       - Claude Code configuration (hooks, skills, agents)
CLAUDE.md      - This session context (auto-loaded)
\`\`\`

## Development Workflow

### When Starting Work

1. Check your context budget: \`echo $STRATAVORE_CONTEXT_TOKEN_BUDGET\`
2. Review project documentation in \`docs/\`
3. Check active runners: \`/stratavore-runners\`
4. If resuming, verify state: \`/verify-recovery-state\`

### During Development

1. Create checkpoints at major milestones: \`/checkpoint "description"\`
2. For investigation work, use subagents: \`use subagents to investigate <topic>\`
3. Emit coordination events when affecting other runners: \`/emit-event "event.name"\`
4. Keep context clean: use \`/clear\` between unrelated tasks

### Before Stopping

1. Create final checkpoint: \`/checkpoint "Final state before stop"\`
2. Verify all tests pass if applicable
3. Summary of progress for next session

## Critical Constraints

- **Context Budget**: $(echo $STRATAVORE_CONTEXT_TOKEN_BUDGET) tokens
- **Protected Files**: .env, .git/, package-lock.json, database configs
- **Token Warning Threshold**: 80% of budget
- **Auto Checkpoint Triggers**: After test runs, successful deployments

## Project Conventions

[Auto-populated from .claude/skills/project-specific-skill.md]

### Code Style

[From docs/CODE-STYLE.md or API.md]

### Architecture Decisions

[From docs/ARCHITECTURE.md]

### Database Schema

[From docs/SCHEMA.md]

## Coordination with Other Runners

This project has $STRATAVORE_PARALLEL_RUNNERS active runners.

Check status: \`/stratavore-runners\`

Key events to listen for:
- \`schema.updated\` - Database schema changed
- \`deployment.ready\` - Deploy pipeline succeeded
- \`tests.failed\` - Test suite failed, coordinate fixes

## Help & Support

- Stratavore docs: \`/stratavore-help\`
- Project architecture: \`docs/ARCHITECTURE.md\`
- API reference: \`docs/API.md\`
- Deployment guide: \`docs/DEPLOYMENT.md\`

CLAUDE_EOF

echo "✓ Stratavore+Claude Code environment initialized"
echo ""
echo "Next steps:"
echo "1. Edit .claude/skills/* with project-specific information"
echo "2. Configure hooks in .claude/settings.json (copy from templates)"
echo "3. Run: stratavore projects $PROJECT_NAME --start"
```

---

## Part 2: Complete Hook Script Library

### Hook 1: Token Quota Enforcement

```bash
#!/bin/bash
# .claude/hooks/quota-check.sh
# Prevents operations when token budget is critically low
# Exit codes: 0=allow, 2=block

INPUT=$(cat)

# Extract event details
TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name // ""')
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // ""')
SESSION_ID=$(echo "$INPUT" | jq -r '.session_id')

# Load Stratavore environment
source ~/.claude/stratavore.env

# Expensive operations that should be blocked if budget is low
EXPENSIVE_PATTERNS=(
  "find.*-type f"      # Recursive finds
  "grep -r"            # Recursive grepping
  "du -sh"             # Full disk usage
  ".*\*\*"             # Glob expansions
  "locate"             # Locate command
)

# Check if this is an expensive operation
IS_EXPENSIVE=false
for pattern in "${EXPENSIVE_PATTERNS[@]}"; do
  if [[ "$TOOL_NAME" == "Bash" ]] && [[ "$COMMAND" =~ $pattern ]]; then
    IS_EXPENSIVE=true
    break
  fi
done

if [ "$IS_EXPENSIVE" = true ]; then
  # Query current token budget
  BUDGET_INFO=$(psql \
    -h "${STRATAVORE_DB_HOST:-localhost}" \
    -U "${STRATAVORE_DB_USER:-stratavore}" \
    -d "${STRATAVORE_DB_NAME:-stratavore_state}" \
    -t -c "
      SELECT json_build_object(
        'total', budget,
        'used', used_tokens,
        'remaining', budget - used_tokens,
        'percent', ROUND(100.0 * used_tokens / budget, 1)
      )
      FROM token_budgets
      WHERE runner_id = '$STRATAVORE_RUNNER_ID'
    " 2>/dev/null)
  
  if [ -z "$BUDGET_INFO" ]; then
    # Database unavailable, allow operation to proceed
    exit 0
  fi
  
  REMAINING=$(echo "$BUDGET_INFO" | jq -r '.remaining')
  PERCENT=$(echo "$BUDGET_INFO" | jq -r '.percent')
  TOTAL=$(echo "$BUDGET_INFO" | jq -r '.total')
  
  # Block if less than 10% remaining
  if (( REMAINING < TOTAL / 10 )); then
    echo "Token budget critical: ${PERCENT}% consumed (${REMAINING} tokens remaining)" >&2
    echo "This expensive operation would exceed your quota." >&2
    echo "Create a checkpoint and request quota extension:" >&2
    echo "  /checkpoint \"Progress so far\"" >&2
    echo "  /stratavore-quota-request 50000" >&2
    exit 2
  fi
  
  # Warn if less than 20% remaining
  if (( REMAINING < TOTAL / 5 )); then
    echo "⚠ Token budget warning: ${PERCENT}% consumed" >&2
  fi
fi

exit 0
```

### Hook 2: Protected Files Guard

```bash
#!/bin/bash
# .claude/hooks/protect-critical.sh
# Prevents accidental modification of critical files

INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')
TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name')

# Files that should never be auto-edited
PROTECTED_ALWAYS=(
  ".env"
  ".env.local"
  "package-lock.json"
  "yarn.lock"
  "pnpm-lock.yaml"
  ".git/"
  ".gitignore"
)

# Files that require special consideration
PROTECTED_CAREFUL=(
  "CLAUDE.md"
  ".claude/settings.json"
  "database.yml"
  "docker-compose.yml"
  "Dockerfile"
  "*.sql"
)

# Check against always-protected list
for pattern in "${PROTECTED_ALWAYS[@]}"; do
  if [[ "$FILE_PATH" == *"$pattern"* ]]; then
    echo "Protected file blocked: $FILE_PATH" >&2
    echo "This file should never be auto-modified." >&2
    exit 2
  fi
done

# For Edit/Write tools on careful files, require explicit confirmation
if [[ "$TOOL_NAME" =~ ^(Edit|Write)$ ]]; then
  for pattern in "${PROTECTED_CAREFUL[@]}"; do
    if [[ "$FILE_PATH" == *"$pattern"* ]]; then
      # In auto mode, block. In interactive mode, ask user.
      if [ "$STRATAVORE_AUTO_MODE" = "true" ]; then
        echo "Cannot modify without confirmation: $FILE_PATH" >&2
        exit 2
      else
        # Return structured JSON to ask for permission
        cat <<EOF
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "ask",
    "permissionDecisionReason": "$FILE_PATH requires explicit confirmation due to critical content"
  }
}
EOF
        exit 0
      fi
    fi
  done
fi

exit 0
```

### Hook 3: Auto-Checkpoint on Milestone

```bash
#!/bin/bash
# .claude/hooks/auto-checkpoint.sh
# Creates checkpoint after successful test runs or deployments

INPUT=$(cat)
TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name')
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // ""')

source ~/.claude/stratavore.env

# Identify milestone operations
CHECKPOINT_TRIGGERS=(
  "npm test"
  "npm run test"
  "pytest"
  "cargo test"
  "go test"
  "npm run deploy"
  "npm run build"
)

# Check if this command should trigger checkpoint
for trigger in "${CHECKPOINT_TRIGGERS[@]}"; do
  if [[ "$COMMAND" == "$trigger"* ]]; then
    # Create checkpoint after successful execution
    CHECKPOINT_NAME="Post-${trigger// /-}-$(date +%s)"
    
    # Call Stratavore API to create checkpoint
    RESPONSE=$(curl -s -X POST \
      "http://localhost:50051/api/v1/runners/$STRATAVORE_RUNNER_ID/checkpoints" \
      -H "Content-Type: application/json" \
      -d "{
        \"description\": \"Automatic checkpoint after: $COMMAND\",
        \"tags\": [\"milestone\", \"${trigger// /-}\"]
      }" 2>/dev/null)
    
    CHECKPOINT_ID=$(echo "$RESPONSE" | jq -r '.id // ""')
    
    if [ -n "$CHECKPOINT_ID" ]; then
      echo "✓ Checkpoint created: $CHECKPOINT_NAME ($CHECKPOINT_ID)" >&2
      
      # Optionally emit event that other runners should know about
      echo "checkpoint.created" | \
        amqp-publish \
          --url="amqp://${STRATAVORE_RABBITMQ_HOST:-localhost}:${STRATAVORE_RABBITMQ_PORT:-5672}" \
          -e "stratavore.events" \
          -r "checkpoint.created.$STRATAVORE_PROJECT_ID" \
          -p \
          2>/dev/null || true
    fi
    
    break
  fi
done

exit 0
```

### Hook 4: Emit Coordination Events

```bash
#!/bin/bash
# .claude/hooks/emit-coordination-event.sh
# Emits events to other runners when significant state changes

INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')
TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name')

source ~/.claude/stratavore.env

# Determine what kind of event to emit based on file changes
EVENT_TYPE=""
EVENT_TAGS=()

if [[ "$FILE_PATH" == *"schema"* ]] || [[ "$FILE_PATH" == *"migration"* ]]; then
  EVENT_TYPE="schema.modified"
  EVENT_TAGS=("schema" "database")
elif [[ "$FILE_PATH" == *"package.json"* ]] || [[ "$FILE_PATH" == *"requirements.txt"* ]]; then
  EVENT_TYPE="dependencies.modified"
  EVENT_TAGS=("dependencies")
elif [[ "$FILE_PATH" == *"dockerfile"* ]] || [[ "$FILE_PATH" == *"docker-compose"* ]]; then
  EVENT_TYPE="infrastructure.modified"
  EVENT_TAGS=("infrastructure" "docker")
elif [[ "$FILE_PATH" == *"api"* ]] && [[ "$TOOL_NAME" =~ ^(Edit|Write)$ ]]; then
  EVENT_TYPE="api.modified"
  EVENT_TAGS=("api")
fi

# Emit event if relevant
if [ -n "$EVENT_TYPE" ]; then
  EVENT_DATA=$(cat <<EOF
{
  "event_type": "$EVENT_TYPE",
  "project_id": "$STRATAVORE_PROJECT_ID",
  "runner_id": "$STRATAVORE_RUNNER_ID",
  "file_path": "$FILE_PATH",
  "timestamp": "$(date -Iseconds)",
  "tags": [$(printf '%s\n' "${EVENT_TAGS[@]}" | jq -R '"\(.)"' | paste -sd ',' -)]
}
EOF
  )
  
  # Publish to RabbitMQ
  echo "$EVENT_DATA" | \
    amqp-publish \
      --url="amqp://${STRATAVORE_RABBITMQ_HOST:-localhost}:${STRATAVORE_RABBITMQ_PORT:-5672}" \
      -e "stratavore.events" \
      -r "$EVENT_TYPE.$STRATAVORE_PROJECT_ID" \
      -p \
      2>/dev/null || true
  
  echo "Event emitted: $EVENT_TYPE" >&2
fi

exit 0
```

### Hook 5: Context Compaction Preservation

```bash
#!/bin/bash
# .claude/hooks/preserve-on-compact.sh
# Injected context on compaction to preserve Stratavore state

# This hook fires on SessionStart with matcher="compact"
# It runs an echo command that outputs context to be re-injected

source ~/.claude/stratavore.env

cat <<'PRESERVATION_EOF'

## Recovery Context (Preserved from Compaction)

**Last Checkpoint**: $STRATAVORE_LAST_CHECKPOINT
**Session ID**: $STRATAVORE_SESSION_ID
**Runner ID**: $STRATAVORE_RUNNER_ID
**Project**: $STRATAVORE_PROJECT_ID
**Context Usage**: $(date +%s) (resuming from checkpoint, use /verify-recovery-state)

### Critical State

Token Budget: Check ~/.claude/stratavore.env for STRATAVORE_CONTEXT_TOKEN_BUDGET

### Next Steps

1. Verify recovery: /verify-recovery-state
2. Check runners: /stratavore-runners
3. Continue from checkpoint: /resume-checkpoint

PRESERVATION_EOF

# Also query recent milestones
echo ""
echo "### Recent Work"
echo ""
psql -h "${STRATAVORE_DB_HOST:-localhost}" \
  -U "${STRATAVORE_DB_USER:-stratavore}" \
  -d "${STRATAVORE_DB_NAME:-stratavore_state}" \
  -t -c "
    SELECT '- ' || description || ' (' || created_at::text || ')'
    FROM checkpoints
    WHERE runner_id = '$STRATAVORE_RUNNER_ID'
    ORDER BY created_at DESC
    LIMIT 5
  " 2>/dev/null || echo "(Checkpoint database unavailable)"

exit 0
```

### Hook 6: Pre-Tool Validation

```bash
#!/bin/bash
# .claude/hooks/pre-tool-validation.sh
# Validates tool calls before execution

INPUT=$(cat)
TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name')
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // ""')

# Prevent dangerous bash commands
DANGEROUS_PATTERNS=(
  "rm -rf /"
  "> /dev/sda"
  "dd if=/dev/zero"
  ":(){ :|:& };:"  # fork bomb
  "curl.*sudo.*sh"
  "wget.*sudo.*sh"
)

if [[ "$TOOL_NAME" == "Bash" ]]; then
  for pattern in "${DANGEROUS_PATTERNS[@]}"; do
    if [[ "$COMMAND" =~ $pattern ]]; then
      echo "Dangerous command blocked: $pattern" >&2
      exit 2
    fi
  done
fi

exit 0
```

---

## Part 3: Complete MCP Server Configuration

### Template: .claude/settings.json with All MCP Servers

```json
{
  "mcpServers": {
    "stratavore-daemon": {
      "command": "stratavore-mcp-server",
      "args": [
        "--socket=/tmp/stratavore.sock",
        "--timeout=30"
      ],
      "env": {
        "STRATAVORE_API_HOST": "${STRATAVORE_DAEMON_HOST:-localhost}",
        "STRATAVORE_API_PORT": "${STRATAVORE_DAEMON_PORT:-50051}"
      },
      "disabled": false,
      "autoStart": true
    },
    "stratavore-postgres": {
      "command": "mcp-postgres",
      "args": [
        "--connection-string=postgresql://${STRATAVORE_DB_USER}:${STRATAVORE_DB_PASSWORD}@${STRATAVORE_DB_HOST}/${STRATAVORE_DB_NAME}"
      ],
      "disabled": false,
      "autoStart": true
    },
    "slack": {
      "command": "mcp-slack",
      "args": ["--bot-token=${SLACK_BOT_TOKEN}"],
      "env": {
        "SLACK_CHANNEL": "#stratavore-events"
      },
      "disabled": false,
      "autoStart": false
    },
    "github": {
      "command": "mcp-github",
      "args": ["--token=${GITHUB_TOKEN}"],
      "env": {
        "GITHUB_REPO": "org/repo"
      },
      "disabled": false,
      "autoStart": false
    }
  },
  "hooks": {
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "echo 'Session started: $STRATAVORE_SESSION_ID'"
          }
        ]
      },
      {
        "matcher": "compact",
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/.claude/hooks/preserve-on-compact.sh"
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/.claude/hooks/protect-critical.sh"
          }
        ]
      },
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/.claude/hooks/pre-tool-validation.sh"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/.claude/hooks/quota-check.sh"
          }
        ]
      },
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/.claude/hooks/auto-checkpoint.sh"
          }
        ]
      }
    ]
  }
}
```

### MCP Tool Usage in Skills

Create skill that uses MCP tools:

```markdown
---
name: stratavore-operations
type: skill
description: MCP-based operations for Stratavore
disabled-for-auto-invocation: false
---

# Stratavore Operations via MCP

## List All Runners

/list-runners

Uses \`stratavore-daemon\` MCP tool:
\`\`\`
stratavore.runners.list(project_id)
\`\`\`

Returns JSON with all runners for this project, their status, token usage.

## Query Token Budget

/check-budget

Uses \`stratavore-postgres\` MCP tool:
\`\`\`sql
SELECT runner_id, budget, used_tokens, budget - used_tokens as remaining
FROM token_budgets
WHERE runner_id = current_runner_id
\`\`\`

## Send Slack Notification

/notify-slack "message"

Uses \`slack\` MCP tool:
\`\`\`
slack.send_message(channel="#stratavore-events", text="message")
\`\`\`

## Get GitHub Info

/github-status

Uses \`github\` MCP tool:
\`\`\`
github.get_repository_status()
\`\`\`
```

---

## Part 4: Skill Templates

### Skill 1: Project Architecture

```markdown
---
name: project-architecture
type: skill
description: This project's architecture and design decisions
context: always-load
---

# Project Architecture

## System Diagram

\`\`\`
┌─────────────┐
│   Clients   │
└──────┬──────┘
       │
┌──────▼──────────────────┐
│   API Layer             │
│  (Express/FastAPI)      │
└──────┬──────────────────┘
       │
┌──────▼──────────────────┐
│   Business Logic        │
│  (Services)             │
└──────┬──────────────────┘
       │
┌──────▼──────────────────┐
│   Data Layer            │
│  (PostgreSQL/MongoDB)   │
└─────────────────────────┘
\`\`\`

## Core Components

### 1. API Layer
- Framework: [Express/FastAPI/etc]
- Base path: /api/v1
- Authentication: JWT bearer tokens
- Error handling: Standardized JSON error responses

### 2. Business Logic
- Service pattern: All business logic in services/
- Dependency injection: [Spring/InversifyJS/etc]
- Error handling: Custom exception hierarchy in errors/

### 3. Data Access
- ORM: [Sequelize/Hibernate/SQLAlchemy]
- Migrations: In migrations/ directory
- Schema: Defined in docs/SCHEMA.md

## Key Design Decisions

### Decision 1: [Describe choice and why]
- **Trade-off**: What was sacrificed
- **When to revisit**: If X happens, reconsider

### Decision 2: [etc]

## Adding New Features

When adding a new feature:

1. Create service in services/
2. Add route in routes/
3. Create migration if schema change
4. Add tests in tests/
5. Update docs/

\`\`\`bash
# Common commands
npm test            # Run test suite
npm run build       # Build project
npm run lint        # Run linter
npm run migrate     # Run migrations
\`\`\`
```

### Skill 2: API Design Guide

```markdown
---
name: api-design
type: skill
description: API design patterns and standards for this project
---

# API Design Standards

## Endpoint Naming Convention

All endpoints follow REST conventions:

\`\`\`
GET    /api/v1/{resource}              # List all
GET    /api/v1/{resource}/{id}         # Get one
POST   /api/v1/{resource}              # Create
PUT    /api/v1/{resource}/{id}         # Update
DELETE /api/v1/{resource}/{id}         # Delete
\`\`\`

Examples:
- GET /api/v1/runners
- GET /api/v1/runners/abc123
- POST /api/v1/runners
- PUT /api/v1/runners/abc123
- DELETE /api/v1/runners/abc123

## Request Format

All requests include:

\`\`\`
Authorization: Bearer <token>
Content-Type: application/json
\`\`\`

## Response Format

Success response (2xx):

\`\`\`json
{
  "data": {...},
  "meta": {
    "timestamp": "2024-02-19T14:32:00Z",
    "request_id": "req_abc123"
  }
}
\`\`\`

Error response (4xx/5xx):

\`\`\`json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable message",
    "details": {...}
  },
  "meta": {
    "timestamp": "2024-02-19T14:32:00Z",
    "request_id": "req_abc123"
  }
}
\`\`\`

## Pagination

List endpoints support pagination:

\`\`\`
GET /api/v1/runners?page=1&limit=20&sort=created_at:desc
\`\`\`

Response includes:

\`\`\`json
{
  "data": [...],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 150,
    "pages": 8
  }
}
\`\`\`

## Error Codes

Standard error codes:

- INVALID_REQUEST: Validation failed
- UNAUTHORIZED: Missing/invalid auth
- FORBIDDEN: Authenticated but not permitted
- NOT_FOUND: Resource doesn't exist
- CONFLICT: Resource already exists (for create)
- INTERNAL_ERROR: Server error
```

### Skill 3: Deployment Guide

```markdown
---
name: deployment-guide
type: skill
description: How to deploy this project
---

# Deployment Guide

## Environments

This project has three environments:

- **Development**: Local/staging, auto-deployed on commit
- **Staging**: Pre-production replica, manual deploy
- **Production**: Live, manual deploy with approvals

## Pre-Deployment Checklist

Before deploying to any environment:

- [ ] All tests pass: \`npm test\`
- [ ] Linter passes: \`npm run lint\`
- [ ] No vulnerable dependencies: \`npm audit\`
- [ ] Database migrations reviewed
- [ ] Changelog updated
- [ ] Version bumped in package.json

## Deployment Commands

### Deploy to Staging

\`\`\`bash
npm run deploy:staging
\`\`\`

This:
1. Builds the project
2. Runs migrations on staging DB
3. Deploys to staging environment
4. Runs smoke tests
5. Reports status

### Deploy to Production

\`\`\`bash
npm run deploy:production
\`\`\`

This:
1. Requires manual approval (will ask)
2. Creates git tag
3. Builds the project
4. Runs migrations on production DB
5. Deploys with blue-green strategy
6. Runs smoke tests
7. Monitors for errors in first 5 minutes

## Rollback

If something goes wrong after deployment:

\`\`\`bash
npm run rollback:production
\`\`\`

This reverts to the previous version.

## Monitoring Post-Deployment

After deploying to production, monitor:

- Error rates: [Link to monitoring]
- Performance: [Link to metrics]
- User reports: [Where to check]

Escalate if error rate > 0.1% or latency p99 > 500ms.
```

---

## Part 5: Agent Definitions

### Agent 1: Investigator Subagent

```json
{
  "name": "investigator",
  "type": "subagent",
  "description": "Isolated subagent for code investigation and research",
  "context": "fork",
  "model": "claude-sonnet-4-20250929",
  "timeout": 300,
  "skills": [
    "project-architecture",
    "api-design",
    "stratavore-operations"
  ],
  "instructions": "You are investigating a codebase in isolation from the main runner. Your goal is to understand and report on a specific topic. Use available tools to explore files, search for patterns, and understand architecture. Return concise findings with key insights, specific file references, and recommendations. Do not include verbose exploration logs - only the distilled findings matter."
}
```

### Agent 2: Verifier Subagent

```json
{
  "name": "verifier",
  "type": "subagent",
  "description": "Code review and verification specialist",
  "context": "fork",
  "model": "claude-opus-4-5-20251101",
  "timeout": 180,
  "instructions": "You are verifying code changes against quality standards. Check for: (1) Performance issues, (2) Security vulnerabilities, (3) Error handling coverage, (4) Backward compatibility, (5) Test coverage, (6) Code style adherence. Return a structured review with checkmarks for passing items and specific recommendations for issues found. Be thorough but concise.",
  "requiredTools": ["read", "bash"]
}
```

### Agent 3: Integration Tester

```json
{
  "name": "integration-tester",
  "type": "subagent",
  "description": "Integration testing and cross-feature validation",
  "context": "fork",
  "model": "claude-sonnet-4-20250929",
  "timeout": 600,
  "instructions": "You are testing integration between components. Run the full test suite, then manually test scenarios that exercise the changes. Check for: (1) No new test failures, (2) No performance regressions, (3) No broken cross-feature interactions, (4) Smooth upgrade path. Return test results and any issues found.",
  "requiredTools": ["bash", "read"]
}
```

---

## Part 6: Environment File Template

Create `~/.claude/stratavore.env` (injected by daemon):

```bash
# Stratavore Session Configuration
# Injected by Stratavore daemon on session start

# Session Identity
STRATAVORE_SESSION_ID="sess_abc123xyz"
STRATAVORE_PROJECT_ID="proj_myproject"
STRATAVORE_RUNNER_ID="runner_def456uvw"
STRATAVORE_DAEMON_HOST="localhost"
STRATAVORE_DAEMON_PORT="50051"

# Context and Token Management
STRATAVORE_CONTEXT_TOKEN_BUDGET=180000
STRATAVORE_CONTEXT_RESERVE=20000
STRATAVORE_CONTEXT_USAGE=45000
STRATAVORE_CONTEXT_PERCENT_USED="25"

# Session Resumption Info
STRATAVORE_IS_RESUMPTION="false"
STRATAVORE_LAST_CHECKPOINT="2024-02-19T13:45:00Z"
STRATAVORE_LAST_CHECKPOINT_ID="ckpt_xyz789"

# Parallel Execution
STRATAVORE_PARALLEL_RUNNERS=3
STRATAVORE_AUTO_MODE="true"

# Database Configuration
STRATAVORE_DB_HOST="localhost"
STRATAVORE_DB_PORT="5432"
STRATAVORE_DB_NAME="stratavore_state"
STRATAVORE_DB_USER="stratavore"
STRATAVORE_DB_PASSWORD="[loaded from secure store]"

# RabbitMQ Configuration
STRATAVORE_RABBITMQ_HOST="localhost"
STRATAVORE_RABBITMQ_PORT="5672"
STRATAVORE_RABBITMQ_USER="guest"
STRATAVORE_RABBITMQ_PASS="guest"
STRATAVORE_RABBITMQ_VHOST="/"

# Event Configuration
STRATAVORE_EVENT_EXCHANGE="stratavore.events"
STRATAVORE_EVENT_PREFIX="$STRATAVORE_PROJECT_ID"

# External Integrations
SLACK_BOT_TOKEN="[loaded from secure store]"
GITHUB_TOKEN="[loaded from secure store]"

# Logging and Debugging
STRATAVORE_LOG_LEVEL="info"
STRATAVORE_DEBUG="false"
```

---

## Part 7: Quick Start Script for New Agent

```bash
#!/bin/bash
# scripts/start-new-runner.sh
# Called by Stratavore daemon when creating new runner

PROJECT_DIR="$1"
RUNNER_ID="$2"

echo "Starting new Stratavore Claude Code runner..."
echo "Project: $PROJECT_DIR"
echo "Runner: $RUNNER_ID"

# Load project
cd "$PROJECT_DIR"

# Verify .claude/ structure exists
if [ ! -f ".claude/settings.json" ]; then
  echo "Error: .claude/settings.json not found"
  echo "Run: scripts/init-stratavore-project.sh to initialize"
  exit 1
fi

# Start Claude Code in auto mode
stratavore launch "$RUNNER_ID" --project "$PROJECT_DIR" --auto

# Claude Code will:
# 1. Load CLAUDE.md (session context)
# 2. Load .claude/settings.json (hooks, MCP, skills)
# 3. Inject ~/.claude/stratavore.env
# 4. Fire SessionStart hooks
# 5. Load skills
# 6. Begin accepting prompts

echo "✓ Runner started: $RUNNER_ID"
```

---

## Checklist: Enabling Stratavore+Claude Code Integration

Use this checklist when setting up a new project:

- [ ] Clone project with `--recurse-submodules` if it has stratavore-ui
- [ ] Run `scripts/init-stratavore-project.sh` to bootstrap
- [ ] Copy hook scripts from `templates/hooks/` to `.claude/hooks/`
- [ ] Make hooks executable: `chmod +x .claude/hooks/*.sh`
- [ ] Copy skill templates from `templates/skills/` to `.claude/skills/`
- [ ] Customize CLAUDE.md with project-specific conventions
- [ ] Configure `.claude/settings.json` with hooks and MCP servers
- [ ] Define custom agents in `.claude/agents/` if needed
- [ ] Test hook execution: `claude --debug` and verify hook output
- [ ] Test MCP connections: Verify each MCP server starts correctly
- [ ] Create initial checkpoint: `/checkpoint "Initial state"`
- [ ] Document any project-specific behaviors in CLAUDE.md
- [ ] Commit `.claude/` directory to version control (except `.local.json`)

---

## Troubleshooting Quick Reference

### Hooks Not Firing

```bash
# Verify hooks are registered
/hooks  # Shows all registered hooks

# Check hook syntax
bash -n .claude/hooks/my-hook.sh

# Test hook manually
echo '{"tool_name":"Bash","tool_input":{"command":"ls"}}' | ./.claude/hooks/my-hook.sh
echo $?  # Check exit code
```

### MCP Server Not Connecting

```bash
# Check MCP server is running
ps aux | grep mcp-

# View MCP logs
tail -f ~/.claude/mcp.log

# Restart MCP in Claude Code
/restart-mcp
```

### Token Budget Miscalculated

```bash
# Check what Claude Code thinks
echo $STRATAVORE_CONTEXT_TOKEN_BUDGET

# Query actual budget
psql stratavore_state -c "SELECT * FROM token_budgets WHERE runner_id='$STRATAVORE_RUNNER_ID'"
```

### Skills Not Loading

```bash
# List available skills
/skills list

# Force reload
/clear
# Then start fresh - skills will reload
```

---

## Conclusion

This reference implementation provides:

1. **Complete hook library**: Copy, adapt, and use immediately
2. **MCP configuration**: Stratavore daemon, PostgreSQL, Slack, GitHub
3. **Skill templates**: Architecture, API design, deployment
4. **Agent definitions**: Investigator, verifier, integration tester
5. **Bootstrap scripts**: Initialize new projects automatically
6. **Troubleshooting guide**: Resolve common issues quickly

For agents in auto mode, this creates a foundation for reliable, coordinated, context-aware development workflows that span multiple runners and projects.
