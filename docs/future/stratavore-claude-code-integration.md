# Stratavore + Claude Code Integration Implementation Guide

## Overview

This guide defines how Stratavore runners integrate with Claude Code's extension system (hooks, skills, subagents, CLAUDE.md, and MCP) to create a cohesive, self-aware AI development workspace. It provides implementation patterns, architectural decisions, and operational guidance for agents working in auto mode.

## Core Integration Principles

### 1. Session Awareness and Context Preservation

Claude Code sessions running under Stratavore must be aware they're part of a larger orchestration system. This enables:

- **Session resumption** across different projects without context loss
- **Cross-project state awareness** so agents understand global constraints
- **Graceful context handoff** between parallel subagents
- **Automatic recovery** from interruptions with full state

#### Implementation Pattern

Store session metadata in `CLAUDE_ENV_FILE` that Stratavore injects:

```bash
# ~/.claude/stratavore.env (injected by Stratavore daemon)
STRATAVORE_SESSION_ID="sess_abc123xyz"
STRATAVORE_PROJECT_ID="proj_myproject"
STRATAVORE_RUNNER_ID="runner_def456"
STRATAVORE_CONTEXT_TOKEN_BUDGET=120000
STRATAVORE_CONTEXT_RESERVE=20000
STRATAVORE_IS_RESUMPTION="true"
STRATAVORE_LAST_CHECKPOINT="2024-02-19T14:32:00Z"
STRATAVORE_PARALLEL_RUNNERS=3
```

This allows hooks and skills to make decisions based on global state:

```bash
# Hook example: Check if we're in a resumption
if [ "$STRATAVORE_IS_RESUMPTION" = "true" ]; then
  echo "Resuming from checkpoint: $STRATAVORE_LAST_CHECKPOINT"
  # Re-inject context from checkpoint
fi
```

### 2. Deterministic Automation with Hooks

Hooks replace probabilistic LLM decisions with deterministic rules, critical for reliable multi-runner orchestration.

#### Use Hooks For

✓ **Always-enforce rules**: Protected files, quota checks, permission gates
✓ **Predictable automation**: Code formatting, linting, validation
✓ **External integration**: Notifications, metrics publication, event dispatch
✓ **State synchronization**: Checkpoint creation, session snapshots

#### Don't Use Hooks For

✗ Judgment calls that vary by context (use prompt-based hooks instead)
✗ Long-running validations (use agent-based hooks instead)
✗ Feature-specific logic (put in skills/CLAUDE.md instead)

### 3. Context Isolation with Subagents

When a runner spawns subagents for parallel work (investigation, testing, verification), keep main conversation clean.

#### Pattern: Investigation Subagent

```bash
# In main runner's CLAUDE.md
use subagents to investigate <topic>
# Returns:
# - Summary of findings
# - Key decisions to make
# - Blockers or dependencies
```

Main conversation stays focused on implementation while subagent does research.

#### Pattern: Verification Subagent

```bash
# After implementation, spawn verification in isolated context
use a subagent to verify the changes against:
- Performance requirements
- Security guidelines
- Error handling coverage
- Backward compatibility
```

Verification findings feed back as additional context without cluttering main flow.

### 4. Reusable Knowledge with Skills

Skills enable sharing tested workflows and domain knowledge across runners without duplicating context.

#### Global Skills (All Runners)

Place in `/etc/stratavore/skills/` (Stratavore installs global skills):

```
/etc/stratavore/skills/
├── stratavore-cli/              # Stratavore CLI patterns
├── runner-coordination/          # Multi-runner patterns
├── quota-aware-development/     # Token/resource budgeting
├── event-sourcing-patterns/     # Event-driven patterns
└── docker-gantry-integration/   # Docker/Gantry specifics
```

#### Project-Specific Skills

Place in `.claude/skills/` (checked into repo):

```
.claude/skills/
├── api-design/                  # Project's API style guide
├── architecture/                # System architecture overview
├── data-model/                  # Schema and relationships
└── deployment/                  # Project deployment procedures
```

When a new runner is created for a project, these skills auto-load.

#### Skill Frontmatter for Stratavore

```yaml
---
name: stratavore-coordination
type: skill
description: Patterns for multi-runner coordination
disabled-for-auto-invocation: false
preload-on-session-start: true
context-cost: low
prerequisites:
  - understanding of runners
  - familiarity with PostgreSQL queries
---

# Stratavore Coordination Patterns

## Runner-to-Runner Communication

Use the event bus to coordinate:

```bash
# Emit event that other runners can listen for
/emit-event "deployment.approved" --data '{"version":"1.0.0"}'
```

## Token Budget Awareness

Always check available tokens before large operations:

\`\`\`bash
BUDGET=$(cat ~/.claude/stratavore.env | grep STRATAVORE_CONTEXT_TOKEN_BUDGET)
\`\`\`

... rest of skill content
```

### 5. Always-On Context with CLAUDE.md

CLAUDE.md is the persistent context that runs in every session. It's additive across the runner hierarchy.

#### Stratavore Runner CLAUDE.md Structure

```markdown
# Stratavore-Aware Development Context

## Session Identity
- Runner ID: $STRATAVORE_RUNNER_ID
- Project: $STRATAVORE_PROJECT_ID
- Session ID: $STRATAVORE_SESSION_ID

## Global Constraints

### Context Budget
- Total window: 200,000 tokens
- Reserved for recovery: 20,000 tokens
- Available: 180,000 tokens
- Strategy: Run /clear between unrelated tasks; use subagents for investigation

### Token Quotas
- Project budget: Check $STRATAVORE_PROJECT_TOKEN_BUDGET
- Current usage: Query stratavore_state.token_budgets
- If approaching limit: Stop and request extension via /stratavore-quota-request

## Project Conventions
- [Auto-load from .claude/skills/ in this project]

## Runner Coordination
- Parallel runners: $STRATAVORE_PARALLEL_RUNNERS
- Use event bus for coordination (see stratavore-coordination skill)
- Check runner status: /stratavore-status

## Checkpointing
- Last checkpoint: $STRATAVORE_LAST_CHECKPOINT
- Create checkpoint when: Major milestone reached, context compaction triggered
- Command: /stratavore-checkpoint "description of progress"

## Recovery Protocol
If interrupted:
1. Esc twice to open rewind menu
2. Identify last good state
3. /stratavore-resume from checkpoint
4. Verify state consistency with /stratavore-state-check

## When Spawning Subagents
- Provide subagent with: Task description, what to research, constraints
- Expected return: Summary findings, blockers, next steps
- Do NOT include implementation details in main conversation

## Auto Mode Behavior
This runner is operating in auto mode. Follow these rules:
- Use hooks for enforcement, not requests
- Prefer deterministic patterns over probabilistic ones
- When uncertain, ask for human input via /ask-user
- Create checkpoints every 30 minutes of work
- Monitor event bus for coordination signals

## File Organization
```
project/
├── CLAUDE.md (this file)
├── .claude/
│   ├── settings.json (hooks, MCP config)
│   ├── skills/ (project-specific skills)
│   ├── hooks/ (custom hook scripts)
│   └── agents/ (custom subagent definitions)
├── docs/
│   ├── ARCHITECTURE.md
│   ├── API.md
│   └── SCHEMA.md
└── [project files]
```

See the documentation files for context on architecture, API patterns, and data models.
```

---

## Hook Patterns for Stratavore

### Pattern 1: Quota Enforcement Hook

Prevent operations that would exceed token/resource quotas:

```bash
# .claude/hooks/quota-check.sh
#!/bin/bash

INPUT=$(cat)
TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name')
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

# Check if this is an expensive operation
if [[ "$TOOL_NAME" == "Bash" ]] && [[ "$COMMAND" == *"find"* ]] || [[ "$COMMAND" == *"grep -r"* ]]; then
  # Query token budget
  REMAINING=$(psql -h localhost stratavore_state -tc \
    "SELECT remaining_tokens FROM token_budgets WHERE runner_id = '$STRATAVORE_RUNNER_ID'")
  
  if [ "$REMAINING" -lt 50000 ]; then
    echo "Token budget critically low: ${REMAINING} tokens remaining" >&2
    exit 2  # Block the operation
  fi
fi

exit 0
```

Register in `.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash|Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/.claude/hooks/quota-check.sh"
          }
        ]
      }
    ]
  }
}
```

### Pattern 2: Protected Files Hook

Prevent accidental modification of critical files:

```bash
# .claude/hooks/protect-critical-files.sh
#!/bin/bash

INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')

# Critical patterns that should never be modified
PROTECTED=(
  ".env"
  "STRATAVORE.md"
  "package-lock.json"
  ".git/"
  "migrations/"
  "database.yml"
)

for pattern in "${PROTECTED[@]}"; do
  if [[ "$FILE_PATH" == *"$pattern"* ]]; then
    echo "Protected file: $FILE_PATH matches pattern '$pattern'. Modifications blocked." >&2
    exit 2
  fi
done

exit 0
```

### Pattern 3: Auto-Checkpoint Hook

Create a checkpoint whenever major milestones are reached:

```bash
# .claude/hooks/auto-checkpoint.sh
#!/bin/bash

INPUT=$(cat)
TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name')
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

# Trigger checkpoint after successful test runs or deployments
if [[ "$TOOL_NAME" == "Bash" ]] && [[ "$COMMAND" == *"test"* ]]; then
  # Create checkpoint via Stratavore API
  CHECKPOINT=$(curl -s http://localhost:50051/api/v1/runners/$STRATAVORE_RUNNER_ID/checkpoint \
    -X POST \
    -H "Content-Type: application/json" \
    -d "{\"description\": \"Post-test checkpoint\", \"command\": \"$COMMAND\"}")
  
  echo "Checkpoint created: $(echo $CHECKPOINT | jq -r '.checkpoint_id')" >&2
fi

exit 0
```

### Pattern 4: Event Bus Emission Hook

Emit events to other runners when significant state changes:

```bash
# .claude/hooks/emit-coordination-event.sh
#!/bin/bash

INPUT=$(cat)
TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name')
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')

# Emit event when schema changes
if [[ "$FILE_PATH" == *"schema"* ]] || [[ "$FILE_PATH" == *"migrations"* ]]; then
  RABBIT_HOST="${STRATAVORE_RABBITMQ_HOST:-localhost}"
  RABBIT_PORT="${STRATAVORE_RABBITMQ_PORT:-5672}"
  
  # Publish to event bus (would use rabbitmq-publish CLI or MCP tool)
  echo "schema.updated" | amqp-publish \
    --url="amqp://guest:guest@$RABBIT_HOST:$RABBIT_PORT" \
    -e "stratavore.events" \
    -r "schema.updated.$STRATAVORE_PROJECT_ID"
fi

exit 0
```

---

## MCP Integration for Stratavore

Configure MCP servers in `.claude/settings.json` to give Claude Code access to Stratavore infrastructure:

### Pattern 1: Stratavore Daemon MCP Server

```json
{
  "mcpServers": {
    "stratavore-daemon": {
      "command": "stratavore-mcp-server",
      "args": ["--socket=/tmp/stratavore.sock"],
      "env": {
        "STRATAVORE_API_HOST": "localhost",
        "STRATAVORE_API_PORT": "50051"
      }
    }
  }
}
```

This MCP server exposes tools:

```
stratavore.runners.list()        -> Current active runners
stratavore.runners.status(id)    -> Runner health, context usage
stratavore.events.list(filter)   -> Recent events in event bus
stratavore.projects.quota(id)    -> Token budget and usage
stratavore.checkpoint.create()   -> Create explicit checkpoint
stratavore.checkpoint.load(id)   -> Resume from checkpoint
```

Skills can then use these tools:

```markdown
# Multi-Runner Coordination Skill

/list-all-runners shows active runners and their token usage

Implementation uses stratavore.runners.list() MCP tool to:
1. List all active runners
2. Display their context usage
3. Identify resource-constrained runners
4. Suggest load rebalancing
```

### Pattern 2: PostgreSQL MCP for State Queries

```json
{
  "mcpServers": {
    "stratavore-postgres": {
      "command": "mcp-postgres",
      "args": [
        "--connection-string=postgresql://stratavore:password@localhost/stratavore_state"
      ]
    }
  }
}
```

Skills can query runner state directly:

```sql
-- Via /query-postgres MCP tool
SELECT runner_id, status, context_usage, last_heartbeat
FROM runners
WHERE project_id = $STRATAVORE_PROJECT_ID
ORDER BY last_heartbeat DESC;
```

### Pattern 3: Slack MCP for Notifications

```json
{
  "mcpServers": {
    "slack": {
      "command": "mcp-slack",
      "args": ["--bot-token=xoxb-..."],
      "env": {
        "SLACK_CHANNEL": "#stratavore-events"
      }
    }
  }
}
```

Hooks can send notifications:

```bash
# In a hook that needs to alert about critical events
curl -X POST http://localhost:5000/mcp/slack \
  -d '{
    "channel": "#stratavore-events",
    "message": "Runner '"$STRATAVORE_RUNNER_ID"' approaching token limit"
  }'
```

---

## Subagent Patterns for Stratavore

### Pattern 1: Investigation Subagent with Skill Preload

In main runner's CLAUDE.md:

```markdown
## Parallel Investigation

When you need to research something without consuming main context:

use subagents to investigate <topic>

This spawns an isolated subagent that:
- Loads project-specific skills automatically
- Can read/search the codebase freely
- Returns only summary findings
- Never blocks main work
```

Define custom subagent in `.claude/agents/investigator.json`:

```json
{
  "name": "investigator",
  "type": "subagent",
  "description": "Research-focused subagent for codebase investigation",
  "context": "fork",
  "model": "claude-sonnet-4-20250929",
  "skills": [
    "architecture",
    "api-design",
    "stratavore-coordination"
  ],
  "instructions": "You are investigating a codebase. Use available tools to explore files, search for patterns, and understand architecture. Return concise findings with key insights, not verbose exploration logs.",
  "timeout": 300
}
```

### Pattern 2: Verification Subagent

After implementation, spawn verification in isolated context:

```json
{
  "name": "verifier",
  "type": "subagent",
  "description": "Verification-focused subagent for code review",
  "context": "fork",
  "model": "claude-opus-4-5-20251101",
  "instructions": "Verify the provided code changes against: (1) performance requirements, (2) security guidelines, (3) error handling, (4) backward compatibility, (5) test coverage. Return a structured report with findings and recommendations.",
  "timeout": 180,
  "requiredTools": ["read", "bash"]
}
```

Main runner uses this in skill:

```markdown
# Code Verification Skill

/verify-implementation

Spawns verifier subagent with current codebase state. Returns:
- ✓/✗ checklist of verification items
- Performance analysis
- Security concerns
- Breaking changes
- Test coverage gaps
```

### Pattern 3: Parallel Multi-Subagent Tasks

For features requiring multiple specialists:

```markdown
# Agent Team Pattern

When building a major feature, spawn multiple specialized subagents:

use agent team to build <feature>:
- architecture subagent: Design the system
- security-reviewer: Check security implications
- performance-analyzer: Profile and optimize
- test-writer: Ensure test coverage

Subagents work in parallel, each returns findings, main runner synthesizes into implementation.
```

---

## Skill Library Structure for Stratavore

Create reusable skills that all runners can leverage:

### Global Skill: Stratavore CLI Operations

```markdown
---
name: stratavore-cli-operations
type: skill
description: Patterns for interacting with Stratavore daemon
---

# Stratavore CLI Operations

## List Active Runners

/stratavore-runners shows all runners for this project

Uses MCP tool stratavore.runners.list() to display:
- Runner ID
- Status (running/paused/terminated)
- Context usage (tokens used / budget)
- Last heartbeat
- Parallel operations

## Check Token Budget

/check-token-budget displays remaining tokens for this runner

Queries stratavore_state.token_budgets for:
- Total budget
- Used tokens
- Remaining tokens
- Percentage consumed

If approaching 80% consumption, triggers warning.

## Create Checkpoint

/checkpoint "description" creates an explicit checkpoint

Saves session state to stratavore_state.sessions table with:
- Timestamp
- Description
- Full context snapshot
- Code state
- Conversation history

## Resume from Checkpoint

/resume-checkpoint [id] restores session from checkpoint

If called without ID, shows recent checkpoints and lets you choose.
Restores full conversation and code state.

## Emit Coordination Event

/emit-event "event.name" --data '{}' publishes to event bus

Other runners listening on that routing key receive the event.
Enables coordination without direct communication.
```

### Global Skill: Multi-Runner Patterns

```markdown
---
name: multi-runner-patterns
type: skill
description: Patterns for coordinating work across multiple runners
---

# Multi-Runner Coordination Patterns

## Parallel Development

When building independent features:

1. Main runner handles feature A
2. /spawn-runner -p feature-b "Build feature B" spawns parallel runner
3. Each runner has isolated context, works independently
4. Use event bus to coordinate when features interact

## Feature Gate Pattern

For coordinating feature availability:

1. Schema change runner: Updates database schema
2. Emits schema.ready event
3. API runner: Listens for schema.ready before deploying changes
4. Prevents incompatible versions running simultaneously

## Integration Testing

After parallel runners complete:

1. Create integration-test runner
2. Runs full test suite against combined changes
3. Reports back: all green? deploy. conflicts? coordinate fixes.

## Load Rebalancing

If main runner context is full:

1. /stratavore-rebalance suggests which tasks to move to new runners
2. You approve the split
3. New runner spawned with relevant files and skills
4. Seamless handoff with checkpoint
```

### Project-Specific Skill: API Design

```markdown
---
name: api-design
type: skill
description: This project's API style guide and patterns
context: always-load
---

# API Design Guide

[Import from docs/API.md]

## Endpoint Naming

All endpoints follow: `/api/v1/{resource}/{action}`

Examples:
- GET /api/v1/runners/list
- POST /api/v1/projects/create
- PUT /api/v1/runners/{id}/status

## Error Responses

All errors return JSON:
\`\`\`json
{
  "error": "descriptive message",
  "code": "ERROR_CODE",
  "timestamp": "2024-02-19T14:32:00Z"
}
\`\`\`

## Authentication

All requests require header: `Authorization: Bearer <token>`

[... rest of API guide]
```

---

## Context Management Strategy

For auto-mode agents running long sessions, aggressive context management is critical.

### Strategy 1: Subagent Delegation

For investigation/research tasks that consume context:

```
Main Runner (180k context)
├── Task 1: Implementation (core work)
├── Subagent A: Investigation (isolated, 120k)
└── Subagent B: Verification (isolated, 120k)
```

Subagents complete and return summaries. Main runner stays clean.

### Strategy 2: Skill-Based Context Injection

Load skills only when needed (not upfront):

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "prompt",
            "prompt": "The user is asking about: ${USER_PROMPT}. Which skills from .claude/skills/ are relevant? Load only those."
          }
        ]
      }
    ]
  }
}
```

### Strategy 3: Context Compaction with Preservation

When context compaction triggers, ensure critical Stratavore state survives:

```markdown
# In CLAUDE.md

When compacting, always preserve:
- STRATAVORE_SESSION_ID and runner context
- Recent checkpoint locations
- Active coordination events
- Token budget status
- List of modified files
```

Customize via Stratavore settings:

```yaml
# ~/.config/stratavore/stratavore.yaml
contextManagement:
  compactionPreservations:
    - pattern: "STRATAVORE_*"
      priority: "critical"
    - pattern: "token_budget"
      priority: "high"
    - pattern: "checkpoint"
      priority: "high"
```

### Strategy 4: /clear Between Unrelated Tasks

When runner context gets cluttered:

```bash
# After completing one feature, before starting next
/clear
/stratavore-resume "Continue with feature X"
```

Clears conversation history but preserves code state.

---

## Session Resumption Protocol

When a runner session is interrupted and resumed:

### 1. Environment Injection (Stratavore Daemon)

Daemon injects environment file before resuming:

```bash
export $(cat ~/.claude/stratavore.env | xargs)
stratavore resume runner_abc123
```

### 2. Session Start Hook (Stratavore-Aware)

Hook fires on SessionStart with source="resume":

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "resume",
        "hooks": [
          {
            "type": "command",
            "command": "echo 'Resumed from checkpoint: $STRATAVORE_LAST_CHECKPOINT'"
          }
        ]
      }
    ]
  }
}
```

### 3. CLAUDE.md Context Re-injection

CLAUDE.md automatically re-injects context at session start:

```markdown
# On Resume

Session resumed from checkpoint: $STRATAVORE_LAST_CHECKPOINT

Last progress:
[Checkpoint summary is injected here]

Current token usage: $STRATAVORE_CONTEXT_USAGE / $STRATAVORE_CONTEXT_TOKEN_BUDGET

Continue from where you left off.
```

### 4. Recovery Verification

Agent verifies it can continue safely:

```bash
# Custom skill for recovery verification
/verify-recovery-state

Checks:
- Can access all project files
- Can connect to dependencies (database, etc.)
- Token budget still available
- Event bus still connected
- All runners in expected state
```

---

## Operational Runbooks

### Runbook 1: Adding a New Runner to a Project

1. **User action**: `stratavore projects myproject --add-runner`
2. **Daemon creates**: Runner record in database with unique ID
3. **Daemon spawns**: Claude Code session with Stratavore env injection
4. **CLAUDE.md loads**: Project skills auto-load
5. **Hooks register**: Project hooks from .claude/settings.json
6. **Agent starts**: With context of existing runners and progress

### Runbook 2: Token Budget Exhaustion

1. **Hook detects**: Token budget < 20% remaining
2. **Hook action**: Emits `runner.budget_warning` event
3. **Slack notification**: Via MCP server
4. **Agent action**: Creates checkpoint and pauses
5. **Human action**: Reviews progress, can extend budget
6. **Agent resumes**: From checkpoint with extended budget

### Runbook 3: Coordination Event Handling

1. **Event emitted**: Runner A completes schema migration
2. **Event published**: "schema.ready" to RabbitMQ
3. **Event listened**: Runner B's skill listens for schema.ready
4. **Skill triggers**: /deploy-with-schema-compat runs
5. **Coordination**: Prevents incompatible versions

### Runbook 4: Context Crisis (Session Getting Full)

1. **Context usage**: > 90%
2. **Auto compaction**: Triggered by Claude Code
3. **Preservation**: Stratavore-critical context preserved by hook
4. **Manual action**: Agent uses /clear and starts fresh subagent
5. **Handoff**: Critical state in checkpoint for future reference

---

## Debugging and Observability

### Hook Debugging

Enable verbose mode to see all hook executions:

```bash
# In Claude Code CLI
Ctrl+O  # Toggle verbose mode

# Or run with debug flag
claude --debug

# Check hook output
tail -f ~/.claude/debug.log | grep "hook"
```

### MCP Tool Debugging

MCP tool calls are logged when verbose mode is on:

```bash
# See what MCP tools are being called
grep "mcp_call" ~/.claude/debug.log

# Example output:
# 2024-02-19T14:32:00 mcp_call stratavore.runners.list
# 2024-02-19T14:32:01 mcp_result: [runner_abc, runner_def, runner_ghi]
```

### Subagent Monitoring

Monitor subagents spawned by main runner:

```bash
# In main runner
/stratavore-subagents  # Shows all spawned subagents

# Output:
# Subagent investigator:     RUNNING (4min)
# Subagent verifier:         PENDING (queued)
# Subagent integration-test: COMPLETE (5min)

# View subagent context usage
/stratavore-subagent-status investigator
```

### Stratavore Metrics

Check metrics exposed by daemon:

```bash
# Query Prometheus metrics
curl http://localhost:9091/metrics | grep stratavore

# Key metrics
stratavore_runners_total{status="running"} 3
stratavore_runners_by_project{project="myproject"} 3
stratavore_sessions_total 42
stratavore_tokens_used_total{scope="runner"} 1230456
stratavore_heartbeat_latency_seconds{quantile="0.99"} 0.15
```

---

## Best Practices for Auto-Mode Agents

### 1. Determinism Over Probability

✓ Use hooks for rules that must always apply
✓ Use prompt-based/agent-based hooks for judgment calls that require context
✗ Don't rely on Claude to "probably" follow a pattern

### 2. Context as a Constraint

✓ Treat context window like a token budget
✓ Use subagents for investigation to preserve main context
✓ Create checkpoints frequently
✗ Don't assume unlimited context

### 3. Coordination Over Competition

✓ Use event bus to coordinate between runners
✓ Emit events when you make decisions other runners need to know
✓ Check status of other runners before making conflicting changes
✗ Don't assume you're the only runner working on the project

### 4. Verification Over Assumption

✓ Run /verify-implementation after major changes
✓ Use integration tests to catch cross-runner conflicts
✓ Create explicit checkpoints at milestones
✗ Don't assume changes work just because they compiled

### 5. Clarity Over Cleverness

✓ Use descriptive checkpoint descriptions
✓ Emit clear event names with relevant data
✓ Document why you're using a subagent
✗ Don't create complex multi-step automation without documentation

### 6. Recovery Before Optimization

✓ Make sure /resume-checkpoint works before optimizing
✓ Test /clear workflows before relying on them
✓ Ensure MCP connections are reliable before using them in hooks
✗ Don't optimize until recovery is proven

---

## Future Extensions

### Planned

- **Workflow automation**: DAG-based task scheduling
- **Team collaboration**: Multiple humans + agents in same runner
- **Advanced scheduling**: Resource-aware runner placement
- **Session similarity search**: Find similar past sessions via Qdrant
- **Remote runners**: Multi-node support via Stratavore gRPC

### Integration Points for Future

Hooks can be extended to support:
- Custom decision logic via WebAssembly modules
- Integration with external CI/CD systems
- Policy enforcement from organization admin
- Cost tracking and budgeting

Skills can be extended with:
- Multi-language examples (not just Bash/Python/Node)
- Integration-specific workflows
- Team-aware patterns

---

## Conclusion

This integration creates a self-aware AI development workspace where:

1. **Sessions understand they're distributed**: Stratavore context always available
2. **Automation is deterministic**: Hooks enforce rules, not requests
3. **Work can be isolated**: Subagents handle research without consuming main context
4. **Knowledge is reusable**: Skills shared across all runners
5. **Context is managed**: Aggressive compaction strategy with Stratavore-aware preservation
6. **Coordination is explicit**: Event bus enables awareness without tight coupling
7. **Recovery is first-class**: Checkpoints and resumption are primary operations

For agents operating in auto mode, following these patterns enables reliable, scalable AI development workflows that can span multiple runners, projects, and long sessions with full context preservation.
