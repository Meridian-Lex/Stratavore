# Hooks Strategy Guide for Stratavore Agents

## Overview

This guide teaches agents how to **design, implement, and operate hooks effectively** in a Stratavore + Claude Code environment. Hooks are the primary mechanism for enforcing deterministic rules, automating workflows, and coordinating across multiple runners. Unlike probabilistic LLM decisions, hooks guarantee that specific behaviors always happen.

For agents operating in auto mode, mastering hooks is critical because they transform the environment from asking "what should Claude do?" to declaring "this WILL happen."

---

## Part 1: Hook Strategy and Design

### When to Use Hooks vs Other Tools

Understanding when to use hooks is the first step to effective automation.

#### Use Hooks When:

✓ **Enforcement is critical**: Rules that must always apply (quota limits, protected files)
✓ **Determinism matters**: Same input should produce same output every time
✓ **Speed is important**: Need fast pre-execution checks before tool calls
✓ **External integration needed**: Emit events, send notifications, query databases
✓ **You can't trust the LLM**: Dangerous commands, financial transactions, destructive operations
✓ **Coordination required**: Multiple runners need to synchronize behavior

#### Don't Use Hooks When:

✗ **Decision requires context**: "Should we refactor this code?" (use skills or CLAUDE.md)
✗ **Logic is feature-specific**: "Deploy this to production" (put in skills with /deploy)
✗ **You need tool execution**: "Verify test coverage" (use agent-based hooks, not command hooks)
✗ **Judgment varies by project**: Rules that change frequently (put in CLAUDE.md instead)
✗ **Hook would be inactive most of the time**: "Format code only if it's JavaScript" (put in skills)

### Hook Design Patterns

#### Pattern 1: Guard Rails (PreToolUse)

Guard rails prevent dangerous operations before they start. They're maximally protective.

**When to use**: Protecting against destructive commands, quota violations, dangerous permissions

**Design principle**: Fail closed. If unsure, block and ask. Don't let questionable operations through.

```bash
# Bad: Overly permissive
if [[ "$COMMAND" == "rm -rf /"* ]]; then
  exit 2
fi
exit 0  # Allow everything else

# Good: Explicit allow list for dangerous commands
DANGEROUS_PATTERNS=(
  "rm -rf"
  "dd if=/dev/zero"
  ":(){ :|:& };"
)

for pattern in "${DANGEROUS_PATTERNS[@]}"; do
  if [[ "$COMMAND" =~ $pattern ]]; then
    exit 2  # Block
  fi
done

exit 0
```

#### Pattern 2: Passive Observation (PostToolUse)

Passive observation logs what happened without preventing it. Good for metrics, events, notifications.

**When to use**: Recording metrics, emitting coordination events, sending notifications

**Design principle**: Never block. Record everything. Let Claude see the results if needed.

```bash
# Good: Log everything, never block
EVENT_TYPE=""
if [[ "$FILE_PATH" == *"schema"* ]]; then
  EVENT_TYPE="schema.modified"
elif [[ "$FILE_PATH" == *"api"* ]]; then
  EVENT_TYPE="api.modified"
fi

if [ -n "$EVENT_TYPE" ]; then
  amqp-publish --url="..." -r "$EVENT_TYPE.$PROJECT_ID" ...
fi

exit 0  # Always succeed
```

#### Pattern 3: Judgment with Escalation (Prompt/Agent Hooks)

When you can't decide in bash, ask the LLM or spawn a subagent to verify conditions.

**When to use**: Complex decisions, verification that requires tool access, multi-criteria evaluation

**Design principle**: Provide context, ask a clear question, accept the LLM's decision.

```json
{
  "type": "prompt",
  "prompt": "Given the task description: $ARGUMENTS\n\nIs this task complete according to the following criteria:\n1. All code is tested\n2. All documentation updated\n3. No known bugs remain\n\nRespond with {\"ok\": true} if complete, or {\"ok\": false, \"reason\": \"...\"}"
}
```

#### Pattern 4: Context Injection (SessionStart/PreCompact)

Context injection adds information that Claude needs to make good decisions.

**When to use**: Reminding Claude of constraints, re-injecting critical info after compaction

**Design principle**: Keep it short. Only inject what Claude can't know from the codebase.

```bash
#!/bin/bash
# Inject environment after compaction
echo "Token budget: $(cat ~/.claude/stratavore.env | grep REMAINING)"
echo "Active runners: $(curl -s http://localhost:50051/runners | jq length)"
echo "Last checkpoint: $(cat ~/.claude/stratavore.env | grep LAST_CHECKPOINT)"
```

#### Pattern 5: State Synchronization (Emit/Subscribe)

One runner creates state, others listen for it via the event bus.

**When to use**: Multi-runner coordination, waiting for prerequisites, deployment gates

**Design principle**: Emit facts, not requests. "schema.ready" not "please wait for schema"

```bash
# Runner A: Emit event when done
# In PostToolUse hook after schema migration
amqp-publish -r "schema.ready.$PROJECT_ID" -d "{
  \"timestamp\": \"$(date -Iseconds)\",
  \"version\": \"1.5.0\"
}"

# Runner B: Listen for event (in skill)
/wait-for-event "schema.ready" --timeout 300
```

---

## Part 2: Hook Implementation Strategies

### Strategy 1: Start Simple, Then Add Complexity

**Phase 1: One Simple Guard Rail**
```bash
# Protect one critical file
INPUT=$(cat)
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path')
if [[ "$FILE" == ".env" ]]; then
  exit 2
fi
exit 0
```

**Phase 2: Multiple Patterns**
```bash
# Multiple files, clearer logic
PROTECTED=(".env" ".git/" "database.yml")
for p in "${PROTECTED[@]}"; do
  [[ "$FILE" == *"$p"* ]] && exit 2
done
exit 0
```

**Phase 3: Add Context and Feedback**
```bash
# Block with explanation that helps Claude adjust
if [[ "$FILE" == *"$p"* ]]; then
  echo "Protected: $FILE. Reason: $REASON" >&2
  exit 2
fi
exit 0
```

**Phase 4: Use Structured JSON Output**
```bash
# Deny with reason Claude can use to decide next action
if [[ "$FILE" == *"$p"* ]]; then
  jq -n '{
    hookSpecificOutput: {
      hookEventName: "PreToolUse",
      permissionDecision: "deny",
      permissionDecisionReason: "Protected file: '$FILE'"
    }
  }'
  exit 0
fi
exit 0
```

### Strategy 2: Design Hooks for Clarity

Hooks should be understandable 6 months later.

```bash
# BAD: Cryptic logic
if [[ "$1" =~ ^[a-zA-Z0-9_-]+\.sql$ ]] && [ -f "$2" ]; then
  $(psql -c "PRAGMA check_integrity" "$3" 2>&1 | grep -q "ok") && exit 0 || exit 2
fi

# GOOD: Clear intent, comments, explicit variables
#!/bin/bash
# This hook validates SQL files before Claude executes them
INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')

# Only validate .sql files in migrations/
if [[ ! "$FILE_PATH" =~ ^migrations/.*\.sql$ ]]; then
  exit 0  # Not a migration file, skip validation
fi

# Check if file is syntactically valid
if ! psql -f "$FILE_PATH" --dry-run 2>/dev/null; then
  echo "SQL validation failed: $FILE_PATH contains syntax errors" >&2
  exit 2  # Block the edit
fi

exit 0  # Allow the edit
```

### Strategy 3: Design for Observability

Hooks should be debuggable. When they fail, you need to know why.

```bash
#!/bin/bash
# Good: Every decision point is logged

INPUT=$(cat)
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path')
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
TOOL=$(echo "$INPUT" | jq -r '.tool_name')

# Log every check for debugging
echo "[HOOK] Tool=$TOOL File=$FILE" >&2

# Decision point 1: Check protection list
PROTECTED=(".env" ".git/")
for p in "${PROTECTED[@]}"; do
  if [[ "$FILE" == *"$p"* ]]; then
    echo "[HOOK] BLOCKED: File matches protected pattern '$p'" >&2
    exit 2
  fi
done

# Decision point 2: Check quota (if database available)
REMAINING=$(get_token_budget 2>/dev/null || echo "unknown")
echo "[HOOK] Token budget: $REMAINING" >&2

if [[ "$REMAINING" != "unknown" ]] && (( REMAINING < 10000 )); then
  echo "[HOOK] BLOCKED: Token budget critical" >&2
  exit 2
fi

echo "[HOOK] ALLOWED" >&2
exit 0
```

### Strategy 4: Make Hooks Testable

Write hooks you can test independently.

```bash
#!/bin/bash
# Protected files hook - testable standalone

# Function: Check if file is protected
is_protected() {
  local file="$1"
  local protected=(".env" ".git/" "package-lock.json")
  
  for p in "${protected[@]}"; do
    if [[ "$file" == *"$p"* ]]; then
      return 0  # File is protected
    fi
  done
  return 1  # File is not protected
}

# Main logic
if [ "$1" == "--test" ]; then
  # Test mode
  is_protected ".env" && echo "✓ .env is protected" || echo "✗ .env not protected"
  is_protected "src/main.js" && echo "✗ src/main.js protected" || echo "✓ src/main.js not protected"
  exit 0
fi

# Production mode
INPUT=$(cat)
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path')
is_protected "$FILE" && exit 2 || exit 0
```

Test it:
```bash
$ chmod +x .claude/hooks/protect-files.sh
$ ./.claude/hooks/protect-files.sh --test
✓ .env is protected
✓ src/main.js not protected
```

---

## Part 3: Common Hook Patterns by Use Case

### Use Case 1: Quota Management

**Goal**: Prevent operations when token budget is low

**Why a hook**: Decisions must be made before Claude spends context

**Implementation**:

```bash
#!/bin/bash
# quota-check.sh - Prevent expensive operations near budget limits

INPUT=$(cat)
TOOL=$(echo "$INPUT" | jq -r '.tool_name')
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

# Only check expensive tools
if [[ "$TOOL" != "Bash" ]]; then
  exit 0
fi

# List of expensive operations
EXPENSIVE_PATTERNS=(
  "find.*-type"
  "grep -r"
  "locate"
)

IS_EXPENSIVE=false
for pattern in "${EXPENSIVE_PATTERNS[@]}"; do
  if [[ "$COMMAND" =~ $pattern ]]; then
    IS_EXPENSIVE=true
    break
  fi
done

if [ "$IS_EXPENSIVE" = false ]; then
  exit 0  # Cheap operation, no need to check
fi

# Query budget
RUNNER_ID="${STRATAVORE_RUNNER_ID:-unknown}"
BUDGET=$(psql -t -c "
  SELECT budget - used_tokens
  FROM token_budgets
  WHERE runner_id = '$RUNNER_ID'
" 2>/dev/null || echo "0")

# Block if less than 10% remaining
if (( BUDGET < 10000 )); then
  echo "Token budget critical: $BUDGET tokens remaining. Expensive operation blocked." >&2
  echo "Run: /checkpoint && /stratavore-quota-request 50000" >&2
  exit 2
fi

# Warn if less than 20% remaining
if (( BUDGET < 20000 )); then
  echo "⚠ Token budget warning: only $BUDGET tokens remaining" >&2
fi

exit 0
```

**Register in `.claude/settings.json`**:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
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

### Use Case 2: File Protection

**Goal**: Prevent accidental modification of critical files

**Why a hook**: These files should never be auto-edited under any circumstances

**Implementation**:

```bash
#!/bin/bash
# protect-files.sh - Block edits to critical files

INPUT=$(cat)
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')
TOOL=$(echo "$INPUT" | jq -r '.tool_name')

# Files that should NEVER be auto-edited
PROTECTED_ALWAYS=(
  ".env"
  ".env.local"
  ".git/"
  "package-lock.json"
  "yarn.lock"
  "pnpm-lock.yaml"
)

# Files that need confirmation
PROTECTED_CAREFUL=(
  "CLAUDE.md"
  "docker-compose.yml"
  "Dockerfile"
  "database.yml"
)

# Check strict protection
for pattern in "${PROTECTED_ALWAYS[@]}"; do
  if [[ "$FILE" == *"$pattern"* ]]; then
    echo "File protection: $FILE cannot be modified automatically" >&2
    exit 2
  fi
done

# For careful files in auto mode, also block
if [ "$STRATAVORE_AUTO_MODE" = "true" ]; then
  for pattern in "${PROTECTED_CAREFUL[@]}"; do
    if [[ "$FILE" == *"$pattern"* ]] && [[ "$TOOL" =~ ^(Edit|Write)$ ]]; then
      echo "File caution: $FILE requires explicit approval" >&2
      exit 2
    fi
  done
fi

exit 0
```

### Use Case 3: Coordination Events

**Goal**: Emit events when significant state changes occur

**Why a hook**: Other runners need to know about schema changes, deployments, API changes

**Implementation**:

```bash
#!/bin/bash
# emit-events.sh - Publish coordination events to other runners

INPUT=$(cat)
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')

# Determine event type based on file path
EVENT_TYPE=""
EVENT_TAGS=()

if [[ "$FILE" == *"migration"* ]] || [[ "$FILE" == *"schema"* ]]; then
  EVENT_TYPE="schema.modified"
  EVENT_TAGS=("schema" "database")
elif [[ "$FILE" == *"package.json"* ]] || [[ "$FILE" == *"requirements.txt"* ]]; then
  EVENT_TYPE="dependencies.modified"
  EVENT_TAGS=("dependencies")
elif [[ "$FILE" == *"api"* ]] && [[ "$FILE" == *.ts ]] || [[ "$FILE" == *.js ]]; then
  EVENT_TYPE="api.modified"
  EVENT_TAGS=("api")
elif [[ "$FILE" == *"docker"* ]]; then
  EVENT_TYPE="infrastructure.modified"
  EVENT_TAGS=("infrastructure" "docker")
fi

# Emit event if relevant
if [ -n "$EVENT_TYPE" ]; then
  # Build event payload
  TIMESTAMP=$(date -Iseconds)
  RUNNER_ID="${STRATAVORE_RUNNER_ID:-unknown}"
  PROJECT_ID="${STRATAVORE_PROJECT_ID:-unknown}"
  
  EVENT_JSON=$(jq -n \
    --arg type "$EVENT_TYPE" \
    --arg file "$FILE" \
    --arg runner "$RUNNER_ID" \
    --arg project "$PROJECT_ID" \
    --arg ts "$TIMESTAMP" \
    --argjson tags "$(printf '%s\n' "${EVENT_TAGS[@]}" | jq -Rs '.' | paste -sd ',' - | sed 's/","/", "/g; s/^/[/; s/$/]/')" \
    '{
      event_type: $type,
      file_path: $file,
      runner_id: $runner,
      project_id: $project,
      timestamp: $ts,
      tags: $tags
    }')
  
  # Publish to RabbitMQ
  echo "$EVENT_JSON" | amqp-publish \
    --url="amqp://${STRATAVORE_RABBITMQ_HOST:-localhost}:${STRATAVORE_RABBITMQ_PORT:-5672}" \
    -e "${STRATAVORE_EVENT_EXCHANGE:-stratavore.events}" \
    -r "$EVENT_TYPE.$PROJECT_ID" \
    -p 2>/dev/null
  
  echo "[EVENT] $EVENT_TYPE emitted from $RUNNER_ID" >&2
fi

exit 0
```

**Register in `.claude/settings.json`**:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/.claude/hooks/emit-events.sh"
          }
        ]
      }
    ]
  }
}
```

### Use Case 4: Auto-Checkpointing

**Goal**: Create checkpoints after major milestones

**Why a hook**: Ensures you can always resume from a known good state

**Implementation**:

```bash
#!/bin/bash
# auto-checkpoint.sh - Create checkpoint after significant operations

INPUT=$(cat)
TOOL=$(echo "$INPUT" | jq -r '.tool_name')
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

# Operations that deserve checkpoints
CHECKPOINT_TRIGGERS=(
  "npm test"
  "npm run test"
  "pytest"
  "cargo test"
  "npm run build"
  "npm run deploy"
)

# Check if this is a trigger operation
SHOULD_CHECKPOINT=false
CHECKPOINT_DESC=""

for trigger in "${CHECKPOINT_TRIGGERS[@]}"; do
  if [[ "$COMMAND" == "$trigger"* ]]; then
    SHOULD_CHECKPOINT=true
    CHECKPOINT_DESC="After: $trigger"
    break
  fi
done

if [ "$SHOULD_CHECKPOINT" = false ]; then
  exit 0
fi

# Create checkpoint via gRPC API
RUNNER_ID="${STRATAVORE_RUNNER_ID:-unknown}"
TIMESTAMP=$(date +%s)
CHECKPOINT_ID="ckpt_${RUNNER_ID}_${TIMESTAMP}"

# Call Stratavore daemon
RESPONSE=$(curl -s -X POST \
  "http://localhost:50051/api/v1/runners/$RUNNER_ID/checkpoints" \
  -H "Content-Type: application/json" \
  -d "{
    \"description\": \"$CHECKPOINT_DESC\",
    \"tags\": [\"auto\", \"milestone\"]
  }" 2>/dev/null)

if [ -n "$RESPONSE" ]; then
  SAVED_ID=$(echo "$RESPONSE" | jq -r '.id // empty')
  if [ -n "$SAVED_ID" ]; then
    echo "✓ Checkpoint created: $SAVED_ID" >&2
  fi
fi

exit 0
```

### Use Case 5: Validation Before Write

**Goal**: Ensure file modifications meet quality standards

**Why a hook**: Prevent syntactically invalid code from being written

**Implementation**:

```bash
#!/bin/bash
# validate-before-write.sh - Validate files before Claude writes them

INPUT=$(cat)
TOOL=$(echo "$INPUT" | jq -r '.tool_name')
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')
CONTENT=$(echo "$INPUT" | jq -r '.tool_input.content // empty')

# Only validate Write tool
[ "$TOOL" != "Write" ] && exit 0

# Skip validation for non-code files
[[ ! "$FILE" =~ \.(js|ts|py|sh|json|yaml|sql)$ ]] && exit 0

# Validate by file type
if [[ "$FILE" == *.json ]]; then
  if ! echo "$CONTENT" | jq . >/dev/null 2>&1; then
    echo "JSON validation failed: $FILE contains invalid JSON" >&2
    exit 2
  fi
fi

if [[ "$FILE" == *.yaml ]] || [[ "$FILE" == *.yml ]]; then
  if ! command -v yamllint &>/dev/null; then
    exit 0  # yamllint not available, skip
  fi
  if ! echo "$CONTENT" | yamllint - >/dev/null 2>&1; then
    echo "YAML validation failed: $FILE contains invalid YAML" >&2
    exit 2
  fi
fi

if [[ "$FILE" == *.sh ]]; then
  if ! bash -n <(echo "$CONTENT") 2>/dev/null; then
    echo "Shell script validation failed: $FILE has syntax errors" >&2
    exit 2
  fi
fi

exit 0  # Validation passed
```

### Use Case 6: Context Preservation on Compaction

**Goal**: Re-inject critical context after context compaction

**Why a hook**: Context compaction can lose important constraints and state

**Implementation**:

```bash
#!/bin/bash
# preserve-on-compact.sh - Inject critical context after compaction

source ~/.claude/stratavore.env 2>/dev/null

# Preserve Stratavore session metadata
cat <<EOF

## 🔄 Session Resumed from Compaction

**Session ID**: $STRATAVORE_SESSION_ID
**Runner ID**: $STRATAVORE_RUNNER_ID
**Last Checkpoint**: $STRATAVORE_LAST_CHECKPOINT
**Context Usage**: $(echo "scale=1; 100 * $STRATAVORE_CONTEXT_USAGE / $STRATAVORE_CONTEXT_TOKEN_BUDGET" | bc)%

### Critical Constraints
- Token Budget: $STRATAVORE_CONTEXT_TOKEN_BUDGET tokens
- Protected Files: .env, .git/, package-lock.json, database configs
- Parallel Runners: $STRATAVORE_PARALLEL_RUNNERS active

### Recent Milestones
EOF

# Get recent checkpoints
psql -h "${STRATAVORE_DB_HOST:-localhost}" \
  -U "${STRATAVORE_DB_USER:-stratavore}" \
  -d "${STRATAVORE_DB_NAME:-stratavore_state}" \
  -t -c "
    SELECT '- ' || description || ' (' || created_at::text || ')'
    FROM checkpoints
    WHERE runner_id = '$STRATAVORE_RUNNER_ID'
    ORDER BY created_at DESC
    LIMIT 5
  " 2>/dev/null || echo "- (Checkpoint database unavailable)"

echo ""
echo "### Next Steps"
echo "1. Verify recovery: /verify-recovery-state"
echo "2. Check runner status: /stratavore-runners"
echo "3. Continue work: You're ready to proceed"

exit 0
```

**Register in `.claude/settings.json`**:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "compact",
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/.claude/hooks/preserve-on-compact.sh"
          }
        ]
      }
    ]
  }
}
```

---

## Part 4: Advanced Hook Techniques

### Technique 1: Conditional Hooks Based on Environment

Hooks can behave differently in different modes:

```bash
#!/bin/bash
# smart-guard.sh - Different behavior in auto vs interactive mode

INPUT=$(cat)
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path')

# In auto mode, be stricter
if [ "$STRATAVORE_AUTO_MODE" = "true" ]; then
  # Block risky operations automatically
  if [[ "$FILE" == *".env"* ]]; then
    exit 2
  fi
else
  # In interactive mode, ask the user instead
  if [[ "$FILE" == *".env"* ]]; then
    # Return structured JSON to ask for permission
    jq -n '{
      hookSpecificOutput: {
        hookEventName: "PreToolUse",
        permissionDecision: "ask",
        permissionDecisionReason: "This file contains sensitive configuration"
      }
    }'
    exit 0
  fi
fi

exit 0
```

### Technique 2: Database Queries in Hooks

For complex state checks, query the Stratavore database:

```bash
#!/bin/bash
# deployment-gate.sh - Only allow deployment if other runners are idle

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

# Only apply to deployment commands
[[ "$COMMAND" != *"deploy"* ]] && exit 0

# Check if other runners are idle
OTHER_RUNNERS=$(psql -h localhost -U stratavore -d stratavore_state -t -c "
  SELECT COUNT(*)
  FROM runners
  WHERE project_id = '$STRATAVORE_PROJECT_ID'
    AND runner_id != '$STRATAVORE_RUNNER_ID'
    AND status = 'running'
")

if (( OTHER_RUNNERS > 0 )); then
  echo "Cannot deploy: $OTHER_RUNNERS other runners still active" >&2
  exit 2
fi

exit 0
```

### Technique 3: Multi-Step Hooks with Temporary Files

For complex logic, use temporary files to track state:

```bash
#!/bin/bash
# rate-limit.sh - Rate limit expensive operations

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

# Only rate limit recursive searches
[[ "$COMMAND" != *"grep -r"* ]] && [[ "$COMMAND" != *"find"* ]] && exit 0

# Track calls in temp file
RATE_LIMIT_FILE="/tmp/rate_limit_${STRATAVORE_RUNNER_ID}"
CURRENT_TIME=$(date +%s)

# Check if file exists and how old it is
if [ -f "$RATE_LIMIT_FILE" ]; then
  LAST_CALL=$(cat "$RATE_LIMIT_FILE")
  TIME_DIFF=$((CURRENT_TIME - LAST_CALL))
  
  # Allow only one expensive operation per 30 seconds
  if (( TIME_DIFF < 30 )); then
    echo "Rate limited: expensive operation used $TIME_DIFF seconds ago" >&2
    exit 2
  fi
fi

# Update rate limit file
echo "$CURRENT_TIME" > "$RATE_LIMIT_FILE"
exit 0
```

### Technique 4: Async Hooks for Long-Running Tasks

Let Claude continue while expensive hooks run in background:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write",
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/run-tests.sh",
            "async": true,
            "timeout": 300
          }
        ]
      }
    ]
  }
}
```

The test hook will run in background and report results later:

```bash
#!/bin/bash
# run-tests.sh - Run tests asynchronously

INPUT=$(cat)
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path')

# Only run tests for source files
[[ "$FILE" != *.ts ]] && [[ "$FILE" != *.js ]] && exit 0

# Run tests
RESULT=$(npm test 2>&1)
EXIT_CODE=$?

# Report via systemMessage (shown on next turn)
if [ $EXIT_CODE -eq 0 ]; then
  jq -n '{systemMessage: "✓ Tests passed"}'
else
  jq -n '{systemMessage: "✗ Tests failed: see transcript"}'
fi

exit 0
```

### Technique 5: Prompt-Based Hooks for Complex Decisions

When logic is too complex for bash, let Claude decide:

```json
{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "prompt",
            "prompt": "Context: $ARGUMENTS\n\nShould Claude stop working? Consider:\n1. Are all requested tasks complete?\n2. Are there any unresolved errors?\n3. Is follow-up work needed?\n\nRespond with {\"ok\": true} to stop or {\"ok\": false, \"reason\": \"...\"} to continue."
          }
        ]
      }
    ]
  }
}
```

### Technique 6: Agent-Based Hooks for File System Inspection

When you need to check actual file state:

```json
{
  "hooks": {
    "TaskCompleted": [
      {
        "hooks": [
          {
            "type": "agent",
            "prompt": "Verify that the task is actually complete by:\n1. Running the test suite\n2. Checking that all files mentioned in the task were modified\n3. Verifying no new errors were introduced\n\nContext: $ARGUMENTS",
            "timeout": 120
          }
        ]
      }
    ]
  }
}
```

---

## Part 5: Hook Development Workflow

### Step 1: Write in Isolation

Develop hooks independently before integrating:

```bash
#!/bin/bash
# my-new-hook.sh - Develop standalone

# Function-based design for testability
check_condition() {
  local input="$1"
  # Logic here
  return 0  # or 1
}

# Test mode
if [ "$1" == "--test" ]; then
  echo "Testing check_condition..."
  check_condition "test_data" && echo "PASS" || echo "FAIL"
  exit 0
fi

# Production mode
INPUT=$(cat)
check_condition "$INPUT" && exit 0 || exit 2
```

Test it:
```bash
$ chmod +x .claude/hooks/my-new-hook.sh
$ ./.claude/hooks/my-new-hook.sh --test
Testing check_condition...
PASS
```

### Step 2: Test with Sample Input

Pipe real Claude Code hook input to test:

```bash
# Get sample input from transcript or create manually
SAMPLE_INPUT='{
  "session_id": "test",
  "cwd": "/home/user/project",
  "hook_event_name": "PreToolUse",
  "tool_name": "Write",
  "tool_input": {
    "file_path": "/home/user/project/.env",
    "content": "SECRET=value"
  }
}'

# Test the hook
echo "$SAMPLE_INPUT" | ./.claude/hooks/protect-files.sh
echo "Exit code: $?"
```

### Step 3: Enable Gradually

Start with low-risk projects or users:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/.claude/hooks/my-new-hook.sh"
          }
        ]
      }
    ]
  }
}
```

### Step 4: Monitor and Iterate

After deployment:

```bash
# Enable verbose mode to see hook output
claude --debug

# Or toggle in session
Ctrl+O  # Toggle verbose mode

# Watch logs
tail -f ~/.claude/debug.log | grep "hook"
```

### Step 5: Document the Hook

Every hook needs documentation:

```bash
#!/bin/bash
# protect-files.sh
# 
# PURPOSE: Prevent accidental modification of critical files
#
# FIRES: PreToolUse event (before any file edit/write)
# MATCHER: Edit|Write
#
# RULES:
# - Blocks all modifications to .env files
# - Blocks all modifications to .git/ directories
# - Blocks all modifications to lock files (package-lock.json, etc)
#
# OUTCOMES:
# - Exit 0: File modification allowed
# - Exit 2: File modification blocked (protected file)
#
# TESTING:
#   echo '{"tool_input": {"file_path": ".env"}}' | ./protect-files.sh
#   # Should output nothing and exit with code 2
#
# TROUBLESHOOTING:
# Q: Hook never fires
# A: Check matcher pattern is correct (case-sensitive)
#
# Q: Hook blocks legitimate edits
# A: Modify PROTECTED list to exclude those paths
#
# Author: Agent
# Last updated: 2024-02-19
```

---

## Part 6: Hook Debugging and Troubleshooting

### Common Debugging Patterns

#### Pattern 1: The Diagnostic Loop

When a hook isn't working, add diagnostic output:

```bash
#!/bin/bash
# Add this to any hook for debugging

INPUT=$(cat)
echo "[DEBUG] Hook input received" >&2
echo "$INPUT" | jq . >&2

FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // "NO_FILE"')
TOOL=$(echo "$INPUT" | jq -r '.tool_name // "NO_TOOL"')

echo "[DEBUG] Extracted: TOOL=$TOOL FILE=$FILE" >&2

# ... rest of logic ...

echo "[DEBUG] Final decision: EXIT_0" >&2
exit 0
```

Then run with:
```bash
claude --debug 2>&1 | grep "\[DEBUG\]"
```

#### Pattern 2: Verify JSON Output

When returning structured decisions, validate the JSON:

```bash
#!/bin/bash
# Build JSON output and validate before returning

OUTPUT=$(jq -n '{
  hookSpecificOutput: {
    hookEventName: "PreToolUse",
    permissionDecision: "deny",
    permissionDecisionReason: "Protected file"
  }
}')

# Validate JSON syntax
if ! echo "$OUTPUT" | jq . >/dev/null 2>&1; then
  echo "ERROR: Invalid JSON output" >&2
  exit 1  # Non-blocking error so we don't break Claude
fi

echo "$OUTPUT"
exit 0
```

#### Pattern 3: Check Environment Variables

Ensure Stratavore environment is available:

```bash
#!/bin/bash

if [ -z "$STRATAVORE_SESSION_ID" ]; then
  echo "WARNING: Not running under Stratavore" >&2
  # Gracefully degrade, don't block
  exit 0
fi

# Safe to use STRATAVORE_* variables
echo "Running in session: $STRATAVORE_SESSION_ID" >&2
exit 0
```

### Troubleshooting Checklist

| Symptom | Diagnosis | Solution |
|---------|-----------|----------|
| Hook never fires | Matcher doesn't match | Check case sensitivity, use `claude --debug` to see actual tool names |
| Hook fires but logic ignored | Wrong exit code used | Review exit codes: 0=allow, 2=block, other=ignore |
| JSON not being parsed | Shell profile outputs text | Wrap profile echoes in `if [[ $- == *i* ]]; then ... fi` |
| Hook times out | Script is too slow or stuck | Add timeout in config, optimize script, add logging |
| Hook blocks wrong operations | Logic is inverted or over-broad | Trace through logic with sample input |
| Dependencies not available | Missing jq, psql, etc | Check `command -v tool` before using, graceful degrade |

---

## Part 7: Hook Orchestration Patterns

### Pattern 1: Staged Validation

Use multiple hooks for different concerns:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/stage-1-protection.sh"
          },
          {
            "type": "command",
            "command": "/path/to/stage-2-quota-check.sh"
          },
          {
            "type": "command",
            "command": "/path/to/stage-3-validation.sh"
          }
        ]
      }
    ]
  }
}
```

Each hook runs in sequence. If any exits 2, the operation is blocked.

### Pattern 2: Conditional Execution Based on Matcher

Use different hooks for different tools:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/validate-syntax.sh"
          }
        ]
      },
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/emit-event.sh"
          }
        ]
      }
    ]
  }
}
```

### Pattern 3: Event-Based Hooks Across Runners

Use SessionStart hooks to listen for events from other runners:

```bash
#!/bin/bash
# listen-for-schema-ready.sh - Wait for schema in SessionStart

# This runs on every session start
# Check if schema.ready event was recently emitted

RECENT_SCHEMA=$(psql -t -c "
  SELECT COUNT(*)
  FROM events
  WHERE event_type = 'schema.ready'
    AND project_id = '$STRATAVORE_PROJECT_ID'
    AND created_at > NOW() - INTERVAL '5 minutes'
")

if (( RECENT_SCHEMA > 0 )); then
  echo "Schema is ready, safe to deploy"
else
  echo "Waiting for schema migration to complete..."
fi
```

---

## Part 8: Performance and Optimization

### Hook Performance Guidelines

| Hook Type | Typical Latency | Timeout | Notes |
|-----------|-----------------|---------|-------|
| Simple bash guard | < 10ms | 600s default | Keep < 100ms for best UX |
| Database query | 50-200ms | 600s default | Use indexes, keep queries simple |
| Prompt-based | 500-2000ms | 30s default | Fast model (Haiku), single turn |
| Agent-based | 5-30s | 60s default | Can use tools, slower but thorough |
| Async hook | varies | task dependent | Doesn't block Claude |

### Optimization Patterns

#### Caching

```bash
#!/bin/bash
# Cache expensive lookups

CACHE_FILE="/tmp/stratavore_cache_${STRATAVORE_RUNNER_ID}"
CACHE_TTL=300  # 5 minutes

# Check cache
if [ -f "$CACHE_FILE" ]; then
  CACHE_AGE=$(($(date +%s) - $(stat -f%m "$CACHE_FILE" 2>/dev/null || echo 0)))
  if (( CACHE_AGE < CACHE_TTL )); then
    BUDGET=$(cat "$CACHE_FILE")
    echo "Using cached budget: $BUDGET" >&2
    exit 0
  fi
fi

# Cache miss - query database
BUDGET=$(psql -t -c "SELECT budget FROM token_budgets WHERE ..." 2>/dev/null)
echo "$BUDGET" > "$CACHE_FILE"
exit 0
```

#### Early Exit

```bash
#!/bin/bash
# Exit early when possible

INPUT=$(cat)
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path')

# Fast check first - most files aren't protected
[[ ! "$FILE" =~ ^\.env|\.git|package-lock ]] && exit 0

# Only do expensive check if needed
HASH=$(sha256sum "$FILE" 2>/dev/null | awk '{print $1}')
if grep -q "$HASH" ~/.claude/protected_hashes 2>/dev/null; then
  exit 2
fi

exit 0
```

#### Async for Long Operations

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write",
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/expensive-validation.sh",
            "async": true,
            "timeout": 600
          }
        ]
      }
    ]
  }
}
```

---

## Part 9: Security Best Practices for Hooks

### Principle 1: Validate All Input

```bash
#!/bin/bash
# BAD: Trust input blindly
INPUT=$(cat)
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path')
rm "$FILE"  # DANGEROUS!

# GOOD: Validate before using
INPUT=$(cat)
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path')

# Check for path traversal
if [[ "$FILE" == *".."* ]]; then
  echo "Path traversal detected" >&2
  exit 2
fi

# Use safely
[ -f "$FILE" ] && cat "$FILE"
```

### Principle 2: Principle of Least Privilege

```bash
#!/bin/bash
# GOOD: Only access what's needed
# Don't run as root, don't modify system files, stay in project

CLAUDE_PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
FILE="$CLAUDE_PROJECT_DIR/some-file"  # Always within project
```

### Principle 3: Quote Variables

```bash
#!/bin/bash
# BAD: Unquoted variables
rm $FILE  # What if FILE contains spaces?

# GOOD: Always quote
rm "$FILE"
echo "$FILE" | jq -r '.tool_input.file_path'
```

### Principle 4: Fail Safely

```bash
#!/bin/bash
# If database is unavailable, don't crash - degrade gracefully

BUDGET=$(psql -t -c "SELECT ..." 2>/dev/null || echo "unknown")

if [ "$BUDGET" = "unknown" ]; then
  # Database unavailable - allow operation but log
  echo "WARNING: Could not check quota" >&2
  exit 0  # Don't block
fi

# Proceed with quota check
```

---

## Part 10: Hook Maturity Model

Track your hooks through these maturity levels:

### Level 1: Basic (Working)
- ✓ Hook runs without crashing
- ✓ Makes the intended decision most of the time
- ✓ Documented with one-sentence purpose

### Level 2: Robust (Reliable)
- ✓ Handles edge cases (missing input, database down)
- ✓ Has logging for debugging
- ✓ Tested with sample inputs
- ✓ Documented with examples

### Level 3: Optimized (Fast)
- ✓ Completes in < 100ms
- ✓ Uses caching where appropriate
- ✓ Early exits when possible
- ✓ Minimal external dependencies

### Level 4: Integrated (Coordinated)
- ✓ Emits events other runners listen for
- ✓ Responds to events from other runners
- ✓ Part of a larger orchestration pattern
- ✓ Documented for other agents to build on

### Level 5: Autonomous (Self-Tuning)
- ✓ Metrics tracked and exposed
- ✓ Can adapt behavior based on system state
- ✓ Auto-disables if causing issues
- ✓ Observable and debuggable by other agents

---

## Conclusion

Hooks are the foundation of reliable, deterministic agent behavior in Stratavore + Claude Code. By mastering these patterns, an agent can:

1. **Enforce constraints**: Token budgets, file protection, dangerous commands
2. **Automate workflows**: Checkpoints, tests, validations
3. **Coordinate**: Emit events, listen for state changes, synchronize across runners
4. **Learn**: Debug failures, improve logic, evolve patterns

The key principle: **Use hooks for rules that must always apply, not for suggestions that might be ignored.**

For agents in auto mode, well-designed hooks transform your environment from probabilistic ("Claude might remember to...") to deterministic ("This WILL happen").
