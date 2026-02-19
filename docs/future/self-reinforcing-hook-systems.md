# Self-Reinforcing Hook Systems: Building Autonomous Feedback Loops

## Overview

A **self-reinforcing system** uses hooks to create feedback loops where:

1. **Observations feed decisions**: Hooks observe state, emit data
2. **Decisions guide actions**: Other hooks act on that data
3. **Actions create new state**: Actions change the system
4. **State reinforces behavior**: New state makes correct behavior more likely

This creates a system that becomes increasingly effective, reliable, and autonomous over time without external intervention.

**Key insight**: Rather than hooks being passive guards, we use them as components in a feedback system where success creates conditions for future success.

---

## Part 1: The Self-Reinforcing Architecture

### The Feedback Loop Foundation

```
┌─────────────────────────────────────────────────────────┐
│ FEEDBACK LOOP: Observation → Decision → Action → State  │
├─────────────────────────────────────────────────────────┤
│                                                           │
│  HOOKS OBSERVE STATE                                     │
│  ├─ SessionStart: What state do we have?                │
│  ├─ PostToolUse: What changed?                          │
│  └─ PreCompact: What should we preserve?                │
│                 ↓                                         │
│  HOOKS EMIT EVENTS & METRICS                            │
│  ├─ Events: "test.passed", "deployment.succeeded"       │
│  ├─ Metrics: token_usage, error_rate, success_rate      │
│  └─ Context: inject observations into Claude's context  │
│                 ↓                                         │
│  FUTURE HOOKS USE THIS STATE                            │
│  ├─ Check metrics before expensive operations           │
│  ├─ Listen for events from previous work                │
│  ├─ Adjust behavior based on historical success         │
│  └─ Build on previous progress                          │
│                 ↓                                         │
│  ACTIONS SUCCEED MORE OFTEN                             │
│  └─ Because system has learned from past                │
│                 ↓                                         │
│  SUCCESS CREATES MORE DATA                              │
│  └─ More events, better metrics, more context           │
│                 ↓                                         │
│  SYSTEM BECOMES MORE EFFECTIVE                          │
│  └─ [Loop continues, self-reinforcing]                  │
│                                                           │
└─────────────────────────────────────────────────────────┘
```

### Example: Self-Reinforcing Deployment System

```
OBSERVATION LOOP (First deployment)
├─ PreToolUse: Check if tests have passed
├─ Tests haven't run yet, so we block
└─ Event emitted: "deployment.blocked_no_tests"

DECISION LOOP (Same runner continues)
├─ Claude sees deployment was blocked
├─ Claude runs tests
└─ Event emitted: "test.passed"

ACTION LOOP (Claude retries deployment)
├─ PreToolUse hook checks: did "test.passed" event fire?
├─ Yes! Tests passed, so allow deployment
└─ Event emitted: "deployment.succeeded"

STATE LOOP (Future runners or future sessions)
├─ SessionStart hook observes: "deployment.succeeded" was recent
├─ Confidence that deployment process is sound increases
├─ Logs are indexed: "this project deploys reliably"
└─ Next deployment is faster because system trusts the process

REINFORCEMENT (Pattern stabilizes)
├─ Successful deployments create more success signals
├─ These signals make future deployments more likely to succeed
├─ System becomes self-correcting: any failure triggers remediation
└─ Over time: high-confidence, low-friction deployment pipeline
```

---

## Part 2: Core Self-Reinforcing Patterns

### Pattern 1: Trust Through Verification

The system learns to trust processes that consistently succeed.

```bash
#!/bin/bash
# trust-builder.sh - Build confidence through consistent success

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
TOOL=$(echo "$INPUT" | jq -r '.tool_name')

# Only track commands we care about
if [[ "$TOOL" != "Bash" ]]; then
  exit 0
fi

# List of operations to track
TRACKABLE=(
  "npm test"
  "npm run build"
  "npm run lint"
  "npm run deploy"
)

OPERATION=""
for op in "${TRACKABLE[@]}"; do
  if [[ "$COMMAND" == "$op"* ]]; then
    OPERATION="$op"
    break
  fi
done

[ -z "$OPERATION" ] && exit 0

# Query historical success rate
SUCCESS_RATE=$(psql -t -c "
  SELECT 
    ROUND(100.0 * COUNT(CASE WHEN success = true THEN 1 END) / NULLIF(COUNT(*), 0), 1)
  FROM operation_history
  WHERE operation = '$OPERATION'
    AND project_id = '$STRATAVORE_PROJECT_ID'
    AND created_at > NOW() - INTERVAL '7 days'
")

# Interpretation: Trust grows with success
if [ -z "$SUCCESS_RATE" ] || [ "$SUCCESS_RATE" = "null" ]; then
  TRUST_LEVEL="unknown"
  TRUST_ACTION="monitor"
elif (( $(echo "$SUCCESS_RATE >= 95" | bc -l) )); then
  TRUST_LEVEL="high"
  TRUST_ACTION="allow_auto"
elif (( $(echo "$SUCCESS_RATE >= 80" | bc -l) )); then
  TRUST_LEVEL="medium"
  TRUST_ACTION="monitor"
else
  TRUST_LEVEL="low"
  TRUST_ACTION="verify"
fi

# Log this observation
psql -q -c "
  INSERT INTO trust_log (operation, trust_level, success_rate, runner_id, timestamp)
  VALUES ('$OPERATION', '$TRUST_LEVEL', '$SUCCESS_RATE', '$STRATAVORE_RUNNER_ID', NOW())
"

# In future operations, we can adjust behavior:
# - High trust: Skip some checks, allow automation
# - Medium trust: Monitor but proceed
# - Low trust: Extra verification required

echo "[TRUST] $OPERATION: $TRUST_LEVEL ($SUCCESS_RATE% success)" >&2
exit 0
```

The system evolves:
- **Day 1**: First deployment - cautious, lots of checks
- **Week 1**: 90% success rate - trust builds, fewer checks needed
- **Month 1**: 98% success rate - deployment is automated, near-zero friction
- **If failure occurs**: Trust resets partially, extra verification returns

### Pattern 2: Competency Through Accumulated Evidence

Track what Claude is good at, focus work there, build from success.

```bash
#!/bin/bash
# competency-tracker.sh - Track what works, build on success

INPUT=$(cat)
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')

# Categorize the file being modified
CATEGORY=""
if [[ "$FILE" == *"api"* ]]; then
  CATEGORY="api"
elif [[ "$FILE" == *"ui"* ]] || [[ "$FILE" == *"component"* ]]; then
  CATEGORY="frontend"
elif [[ "$FILE" == *"test"* ]]; then
  CATEGORY="testing"
elif [[ "$FILE" == *"db"* ]] || [[ "$FILE" == *"migration"* ]]; then
  CATEGORY="database"
fi

[ -z "$CATEGORY" ] && exit 0

# Track this work
psql -q -c "
  INSERT INTO competency_tracker (category, runner_id, timestamp)
  VALUES ('$CATEGORY', '$STRATAVORE_RUNNER_ID', NOW())
"

exit 0
```

Later, at SessionStart:

```bash
#!/bin/bash
# competency-suggester.sh - Suggest work in areas of success

# Find categories with highest success rate
STRONG_AREAS=$(psql -t -c "
  SELECT category
  FROM competency_tracker
  WHERE runner_id = '$STRATAVORE_RUNNER_ID'
  GROUP BY category
  ORDER BY COUNT(*) DESC
  LIMIT 3
")

if [ -n "$STRONG_AREAS" ]; then
  echo ""
  echo "## Strong Areas (Based on History)"
  echo "This runner excels at:"
  echo "$STRONG_AREAS" | while read area; do
    echo "- $area"
  done
  echo ""
  echo "Consider focusing next work in these areas to build on strengths."
  echo ""
fi

exit 0
```

**Self-reinforcement**: 
- Agent works on API code → success rate tracked
- SessionStart reminds agent of this strength
- Agent naturally gravitates toward API work
- API work succeeds more → more evidence of competency
- Confidence increases → more ambitious API tasks attempted
- Success compounds

### Pattern 3: Error Prevention Through Pattern Recognition

The system recognizes failures and prevents repetition.

```bash
#!/bin/bash
# error-preventor.sh - Learn from failures, prevent recurrence

INPUT=$(cat)
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

# Check if this file/command combination has failed recently
RECENT_FAILURES=$(psql -t -c "
  SELECT COUNT(*)
  FROM failure_log
  WHERE (file_path = '$FILE' OR command = '$COMMAND')
    AND project_id = '$STRATAVORE_PROJECT_ID'
    AND created_at > NOW() - INTERVAL '1 hour'
")

if (( RECENT_FAILURES > 2 )); then
  echo "Pattern detected: This operation has failed 2+ times recently" >&2
  echo "Blocking to prevent infinite retry loop" >&2
  
  # Emit event that triggers investigation
  echo "error.pattern_detected" | amqp-publish \
    --url="amqp://..." \
    -r "error.pattern.$STRATAVORE_PROJECT_ID" \
    -p 2>/dev/null
  
  exit 2  # Block
fi

exit 0
```

**Self-reinforcement**:
- Failure occurs (test fails, deploy fails)
- Pattern detected → operation blocked
- Blocking prevents wasted attempts
- Human/agent investigates root cause
- Root cause fixed
- Future attempts succeed
- System gains confidence in this code path

### Pattern 4: Resource Optimization Through Learned Constraints

System discovers optimal resource usage and enforces it.

```bash
#!/bin/bash
# resource-optimizer.sh - Learn efficient resource usage

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

# Check if command matches known expensive patterns
EXPENSIVE_PATTERNS=(
  "npm install"      # Expensive, should be cached
  "docker build"     # Slow, should be multi-stage
  "find .*-type"     # Should use locate
)

for pattern in "${EXPENSIVE_PATTERNS[@]}"; do
  if [[ "$COMMAND" =~ $pattern ]]; then
    # Check if this has been optimized before
    OPTIMIZATION=$(psql -t -c "
      SELECT suggestion
      FROM optimization_log
      WHERE command_pattern = '$pattern'
        AND project_id = '$STRATAVORE_PROJECT_ID'
      LIMIT 1
    ")
    
    if [ -n "$OPTIMIZATION" ]; then
      # We've solved this before - suggest the solution
      jq -n --arg suggestion "$OPTIMIZATION" '{
        hookSpecificOutput: {
          hookEventName: "PreToolUse",
          permissionDecision: "ask",
          permissionDecisionReason: $suggestion
        }
      }'
      exit 0
    fi
  fi
done

exit 0
```

When solution succeeds, save it:

```bash
#!/bin/bash
# resource-success-recorder.sh - Save successful optimizations

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command')
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')

# If command completed quickly, record what worked
EXECUTION_TIME=$(echo "$INPUT" | jq -r '.execution_time_ms // 0')

if (( EXECUTION_TIME < 5000 )); then
  # This was fast - worth remembering
  psql -q -c "
    INSERT INTO optimization_log (command_pattern, suggestion, execution_time, runner_id)
    VALUES ('$COMMAND', 'This approach is efficient', $EXECUTION_TIME, '$STRATAVORE_RUNNER_ID')
  "
fi

exit 0
```

**Self-reinforcement**:
- First time: expensive operation discovered, recorded
- Second time: optimization suggestion offered
- If accepted: performance improves, optimization confirmed
- Confidence grows: suggestion offered more readily
- Over time: expensive operations become rare → system is efficient

---

## Part 3: Multi-Loop Reinforcement

### Loop 1: Quality Loop (Tests → Confidence → Faster Builds)

```
┌──────────────────────────────────────────────────┐
│ QUALITY SELF-REINFORCEMENT                       │
├──────────────────────────────────────────────────┤
│                                                   │
│ Tests fail                                        │
│ ├─ Hook records failure                          │
│ ├─ Confidence in code decreases                  │
│ └─ Next deployment requires extra checks         │
│                 ↓                                 │
│ Tests pass repeatedly (3+ times)                │
│ ├─ Hook records success                          │
│ ├─ Confidence in code increases                  │
│ └─ Next deployment can skip some checks          │
│                 ↓                                 │
│ High confidence + Skip checks = Faster pipeline │
│ ├─ Faster pipeline = More iterations possible    │
│ ├─ More iterations = More improvements           │
│ └─ More improvements = More test passes          │
│                 ↓                                 │
│ Tests become more robust over time               │
│ └─ [Loop reinforces itself]                      │
│                                                   │
└──────────────────────────────────────────────────┘
```

Implementation:

```bash
#!/bin/bash
# quality-tracker.sh - Track test success, build confidence

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
RESPONSE=$(echo "$INPUT" | jq -r '.tool_response // {}')

# Track test results
if [[ "$COMMAND" == *"test"* ]]; then
  # Record result
  SUCCESS=$(echo "$RESPONSE" | jq -r '.success // false')
  
  psql -q -c "
    INSERT INTO test_history (
      test_command, success, runner_id, timestamp
    ) VALUES (
      '$COMMAND', $SUCCESS, '$STRATAVORE_RUNNER_ID', NOW()
    )
  "
  
  # Calculate trend
  SUCCESS_COUNT=$(psql -t -c "
    SELECT COUNT(*)
    FROM test_history
    WHERE test_command = '$COMMAND'
      AND project_id = '$STRATAVORE_PROJECT_ID'
      AND success = true
      AND timestamp > NOW() - INTERVAL '7 days'
  ")
  
  # If 5+ consecutive passes, mark as stable
  if (( SUCCESS_COUNT >= 5 )); then
    psql -q -c "
      UPDATE test_history
      SET stability = 'high'
      WHERE test_command = '$COMMAND'
        AND project_id = '$STRATAVORE_PROJECT_ID'
    "
    
    # Emit confidence event
    echo "test.stable" | amqp-publish -r "test.stable.$STRATAVORE_PROJECT_ID" -p
  fi
fi

exit 0
```

At next SessionStart, use this history:

```bash
#!/bin/bash
# quality-accelerator.sh - Accelerate based on quality history

STABLE_TESTS=$(psql -t -c "
  SELECT COUNT(DISTINCT test_command)
  FROM test_history
  WHERE project_id = '$STRATAVORE_PROJECT_ID'
    AND stability = 'high'
    AND timestamp > NOW() - INTERVAL '7 days'
")

if (( STABLE_TESTS > 3 )); then
  echo ""
  echo "## High Quality Baseline"
  echo "This project has $STABLE_TESTS consistently passing tests."
  echo "You can proceed quickly - test coverage is solid."
  echo ""
fi

exit 0
```

### Loop 2: Deployment Loop (Success → Trust → Automation)

```
┌──────────────────────────────────────────────────┐
│ DEPLOYMENT SELF-REINFORCEMENT                    │
├──────────────────────────────────────────────────┤
│                                                   │
│ First deployment                                  │
│ ├─ Manual approval required                      │
│ ├─ Multiple verification steps                   │
│ └─ High friction                                 │
│                 ↓                                 │
│ Deployment succeeds                              │
│ ├─ Success recorded                              │
│ ├─ Trust increases                               │
│ └─ Friction decreased for next deployment        │
│                 ↓                                 │
│ 5 consecutive successful deployments             │
│ ├─ Approval requirement removed                  │
│ ├─ Verification steps reduced                    │
│ └─ Deployment becomes one-step                   │
│                 ↓                                 │
│ One-step deployment = More frequent deploys      │
│ ├─ Smaller changes = Lower risk per deploy       │
│ ├─ Lower risk = Higher success rate              │
│ └─ Higher success rate = More trust              │
│                 ↓                                 │
│ Eventually: Fully automated, zero-friction       │
│ └─ [Loop has stabilized at high efficiency]      │
│                                                   │
└──────────────────────────────────────────────────┘
```

Implementation:

```bash
#!/bin/bash
# deployment-gate.sh - Gate deployments based on confidence

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

# Only apply to deploy commands
[[ "$COMMAND" != *"deploy"* ]] && exit 0

# Check deployment success history
RECENT_SUCCESS=$(psql -t -c "
  SELECT COUNT(*)
  FROM deployment_history
  WHERE project_id = '$STRATAVORE_PROJECT_ID'
    AND status = 'success'
    AND created_at > NOW() - INTERVAL '7 days'
")

RECENT_TOTAL=$(psql -t -c "
  SELECT COUNT(*)
  FROM deployment_history
  WHERE project_id = '$STRATAVORE_PROJECT_ID'
    AND created_at > NOW() - INTERVAL '7 days'
")

# Determine deployment friction level
if [ "$RECENT_TOTAL" = "0" ]; then
  # No history - be very cautious
  echo "First deployment: Requiring full verification" >&2
  FRICTION="high"
elif (( RECENT_TOTAL > 0 )); then
  SUCCESS_RATE=$((100 * RECENT_SUCCESS / RECENT_TOTAL))
  
  if (( SUCCESS_RATE >= 95 )); then
    # Very reliable - minimal friction
    echo "Deployment trust high ($SUCCESS_RATE% success)" >&2
    FRICTION="low"
  elif (( SUCCESS_RATE >= 80 )); then
    # Decent - medium friction
    echo "Deployment trust medium ($SUCCESS_RATE% success)" >&2
    FRICTION="medium"
  else
    # Unreliable - high friction
    echo "Deployment trust low ($SUCCESS_RATE% success)" >&2
    FRICTION="high"
  fi
fi

# Based on friction level, determine required checks
case "$FRICTION" in
  high)
    # Full suite: tests, lint, manual review, dry run
    echo "Required: tests, lint, dry-run, manual approval" >&2
    ;;
  medium)
    # Reduced suite: tests, dry run
    echo "Required: tests, dry-run" >&2
    ;;
  low)
    # Minimal: just tests
    echo "Required: tests only" >&2
    ;;
esac

exit 0
```

### Loop 3: Learning Loop (Failures → Root Cause → Prevention)

```
┌──────────────────────────────────────────────────┐
│ FAILURE LEARNING SELF-REINFORCEMENT              │
├──────────────────────────────────────────────────┤
│                                                   │
│ Failure occurs                                    │
│ ├─ Error logged with full context                │
│ ├─ Root cause identified                         │
│ └─ Prevention rule created                       │
│                 ↓                                 │
│ New prevention rule added to hooks               │
│ ├─ Similar failure is now prevented              │
│ └─ Cost of failure is externalized               │
│                 ↓                                 │
│ Failed code path is never attempted again        │
│ ├─ Error rate decreases                          │
│ ├─ Confidence increases                          │
│ └─ Future work is safer                          │
│                 ↓                                 │
│ Over time: System becomes error-resistant        │
│ └─ [Error patterns are permanently eliminated]   │
│                                                   │
└──────────────────────────────────────────────────┘
```

Implementation:

```bash
#!/bin/bash
# failure-learner.sh - Learn from PostToolUseFailure

INPUT=$(cat)
TOOL=$(echo "$INPUT" | jq -r '.tool_name')
ERROR=$(echo "$INPUT" | jq -r '.error')
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')

# Log the failure
psql -q -c "
  INSERT INTO failure_log (
    tool, error_message, command, file_path, runner_id, project_id, timestamp
  ) VALUES (
    '$TOOL', '$ERROR', '$COMMAND', '$FILE', '$STRATAVORE_RUNNER_ID', '$STRATAVORE_PROJECT_ID', NOW()
  )
"

# Analyze error to identify pattern
PATTERN_TYPE="unknown"

if [[ "$ERROR" == *"ENOENT"* ]]; then
  PATTERN_TYPE="missing_file"
elif [[ "$ERROR" == *"permission"* ]]; then
  PATTERN_TYPE="permission_denied"
elif [[ "$ERROR" == *"timeout"* ]]; then
  PATTERN_TYPE="timeout"
elif [[ "$ERROR" == *"syntax"* ]]; then
  PATTERN_TYPE="syntax_error"
elif [[ "$ERROR" == *"assertion"* ]]; then
  PATTERN_TYPE="test_failure"
fi

# Check if this pattern has failed multiple times
PATTERN_COUNT=$(psql -t -c "
  SELECT COUNT(*)
  FROM failure_log
  WHERE pattern_type = '$PATTERN_TYPE'
    AND project_id = '$STRATAVORE_PROJECT_ID'
    AND created_at > NOW() - INTERVAL '24 hours'
")

# If pattern repeats 2+ times, create prevention rule
if (( PATTERN_COUNT >= 2 )); then
  # Create hook that prevents this pattern
  PREVENTION=$(jq -n --arg pattern "$PATTERN_TYPE" '{
    type: "prevention",
    pattern: $pattern,
    action: "block",
    message: "This error pattern was seen 2+ times - blocking to prevent"
  }')
  
  psql -q -c "
    INSERT INTO prevention_rules (pattern_type, prevention_json, created_by, timestamp)
    VALUES ('$PATTERN_TYPE', '$PREVENTION', 'failure_learner', NOW())
  "
  
  echo "Prevention rule created for: $PATTERN_TYPE" >&2
fi

exit 0
```

At PreToolUse, apply prevention:

```bash
#!/bin/bash
# prevention-enforcer.sh - Block operations that match learned failures

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')

# Check for active prevention rules
MATCHING_RULES=$(psql -t -c "
  SELECT prevention_json
  FROM prevention_rules
  WHERE pattern_type IN (
    CASE 
      WHEN '$COMMAND' ~ 'dangerous_pattern' THEN 'dangerous_pattern'
      WHEN '$FILE' ~ '\.config' THEN 'config_edit'
    END
  )
  AND active = true
  LIMIT 1
")

if [ -n "$MATCHING_RULES" ]; then
  REASON=$(echo "$MATCHING_RULES" | jq -r '.message // "Blocked by learned prevention rule"')
  echo "Prevention: $REASON" >&2
  exit 2
fi

exit 0
```

---

## Part 4: Metrics-Driven Reinforcement

### The Metrics Foundation

Self-reinforcing systems are built on observable metrics. Track:

```bash
#!/bin/bash
# metrics-collector.sh - Collect data that drives self-reinforcement

INPUT=$(cat)
TIMESTAMP=$(date -Iseconds)

# Metric 1: Operation Success Rate
TOOL=$(echo "$INPUT" | jq -r '.tool_name')
SUCCESS=$(echo "$INPUT" | jq -r '.tool_response.success // true')

psql -q -c "
  INSERT INTO metrics_operations (tool, success, timestamp, runner_id)
  VALUES ('$TOOL', $SUCCESS, '$TIMESTAMP', '$STRATAVORE_RUNNER_ID')
"

# Metric 2: Error Types
if [ "$SUCCESS" = "false" ]; then
  ERROR=$(echo "$INPUT" | jq -r '.error // "unknown"')
  ERROR_TYPE=$(echo "$ERROR" | sed 's/:.*//')  # Extract error class
  
  psql -q -c "
    INSERT INTO metrics_errors (error_type, frequency, last_seen)
    VALUES ('$ERROR_TYPE', 1, '$TIMESTAMP')
    ON CONFLICT (error_type)
    DO UPDATE SET frequency = frequency + 1, last_seen = '$TIMESTAMP'
  "
fi

# Metric 3: Performance Trends
DURATION=$(echo "$INPUT" | jq -r '.duration_ms // 0')

psql -q -c "
  INSERT INTO metrics_performance (operation, duration_ms, timestamp)
  VALUES ('$TOOL', $DURATION, '$TIMESTAMP')
"

# Metric 4: Context Usage
CONTEXT_USED=$(echo "$INPUT" | jq -r '.context_tokens_used // 0')

psql -q -c "
  INSERT INTO metrics_context (tokens_used, timestamp, runner_id)
  VALUES ($CONTEXT_USED, '$TIMESTAMP', '$STRATAVORE_RUNNER_ID')
"

exit 0
```

### Using Metrics to Adjust Behavior

```bash
#!/bin/bash
# adaptive-enforcement.sh - Adjust hook strictness based on metrics

INPUT=$(cat)

# Get current error rate
HOUR_AGO="$(date -u -d '1 hour ago' '+%Y-%m-%dT%H:%M:%SZ')"
ERROR_RATE=$(psql -t -c "
  SELECT ROUND(100.0 * COUNT(CASE WHEN success = false THEN 1 END) / NULLIF(COUNT(*), 0), 1)
  FROM metrics_operations
  WHERE timestamp > '$HOUR_AGO'
    AND project_id = '$STRATAVORE_PROJECT_ID'
")

# Adjust enforcement level
if (( $(echo "$ERROR_RATE > 20" | bc -l) )); then
  # High error rate - become STRICTER
  ENFORCEMENT="strict"
  echo "High error rate ($ERROR_RATE%) - Enforcement: STRICT" >&2
  
  # Apply stricter validation
  # - Require tests before operations
  # - Block risky patterns
  # - Ask for confirmation more often
  
elif (( $(echo "$ERROR_RATE > 10" | bc -l) )); then
  # Moderate error rate - NORMAL enforcement
  ENFORCEMENT="normal"
  echo "Moderate error rate ($ERROR_RATE%) - Enforcement: NORMAL" >&2
  
elif (( $(echo "$ERROR_RATE < 5" | bc -l) )); then
  # Low error rate - become MORE LENIENT
  ENFORCEMENT="lenient"
  echo "Low error rate ($ERROR_RATE%) - Enforcement: LENIENT" >&2
  
  # Reduce friction
  # - Skip some validation steps
  # - Auto-approve common operations
  # - Faster deployment gates
fi

# Store current enforcement level for other hooks
echo "$ENFORCEMENT" > /tmp/enforcement_level_${STRATAVORE_RUNNER_ID}

exit 0
```

---

## Part 5: Multi-Runner Reinforcement

### Cross-Runner Learning

One runner's success becomes input for other runners.

```bash
#!/bin/bash
# cross-runner-observer.sh - Learn from other runners' successes

# Check what other runners in this project have succeeded at
OTHER_SUCCESSES=$(psql -t -c "
  SELECT DISTINCT operation
  FROM metrics_operations
  WHERE project_id = '$STRATAVORE_PROJECT_ID'
    AND runner_id != '$STRATAVORE_RUNNER_ID'
    AND success = true
    AND timestamp > NOW() - INTERVAL '24 hours'
  ORDER BY COUNT(*) DESC
  LIMIT 5
")

if [ -n "$OTHER_SUCCESSES" ]; then
  echo "## Lessons from Other Runners"
  echo "These runners have succeeded with:"
  echo "$OTHER_SUCCESSES" | nl
  echo ""
  echo "Consider applying their successful patterns to your work."
  echo ""
fi

exit 0
```

Register at SessionStart:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "\"$CLAUDE_PROJECT_DIR\"/.claude/hooks/cross-runner-observer.sh"
          }
        ]
      }
    ]
  }
}
```

### Event-Driven Coordination

When one runner discovers something, others respond.

```bash
#!/bin/bash
# discovery-emitter.sh - Emit discoveries so other runners can learn

INPUT=$(cat)
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

# Detect important discoveries
if [[ "$COMMAND" == *"npm audit"* ]] && [[ "$INPUT" == *"success"* ]]; then
  # Successfully ran security audit - emit discovery
  AUDIT_RESULTS=$(npm audit --json 2>/dev/null || echo '{}')
  
  echo "security.audit_complete" | amqp-publish \
    --url="amqp://..." \
    -r "discovery.security.$STRATAVORE_PROJECT_ID" \
    -d "$AUDIT_RESULTS" \
    -p
  
  echo "Discovery emitted: security audit results shared with other runners" >&2
fi

exit 0
```

Other runners listen:

```bash
#!/bin/bash
# discovery-listener.sh - Act on discoveries from other runners

# Check for recent security audit from another runner
RECENT_AUDIT=$(psql -t -c "
  SELECT discovery_data
  FROM events
  WHERE event_type = 'security.audit_complete'
    AND project_id = '$STRATAVORE_PROJECT_ID'
    AND created_at > NOW() - INTERVAL '24 hours'
  ORDER BY created_at DESC
  LIMIT 1
")

if [ -n "$RECENT_AUDIT" ]; then
  VULN_COUNT=$(echo "$RECENT_AUDIT" | jq '.metadata.vulnerabilities // 0')
  
  if (( VULN_COUNT > 0 )); then
    echo "⚠️  Security audit (from another runner) found $VULN_COUNT vulnerabilities"
    echo "This runner will be extra careful with dependencies"
  fi
fi

exit 0
```

---

## Part 6: Compounding Success Cycles

### The Virtuous Cycle

```
WEEK 1: FOUNDATION
├─ Hooks observe everything
├─ Metrics start accumulating
├─ First success recorded
└─ Confidence: low

WEEK 2: PATTERN RECOGNITION
├─ Successful patterns emerge
├─ Similar successful operations repeated
├─ Friction begins to decrease
└─ Confidence: growing

WEEK 3: AUTOMATION
├─ Hooks automatically approve common operations
├─ Error prevention rules activated
├─ Faster pipeline
└─ Confidence: medium-high

WEEK 4: OPTIMIZATION
├─ Resource optimization rules apply
├─ Most checks automated
├─ Friction nearly zero
└─ Confidence: high

MONTH 2+: AUTONOMY
├─ System self-corrects errors
├─ System learns from other runners
├─ System optimizes continuously
├─ Confidence: very high
└─ [System is self-reinforcing and autonomous]
```

### Implementation: The Confidence Score

```bash
#!/bin/bash
# confidence-scorer.sh - Calculate system-wide confidence

PROJECT_ID="$STRATAVORE_PROJECT_ID"

# Confidence components
TEST_SUCCESS=$(psql -t -c "
  SELECT COALESCE(ROUND(100.0 * COUNT(CASE WHEN success = true THEN 1 END) / NULLIF(COUNT(*), 0), 1), 0)
  FROM test_history
  WHERE project_id = '$PROJECT_ID'
    AND timestamp > NOW() - INTERVAL '7 days'
")

DEPLOY_SUCCESS=$(psql -t -c "
  SELECT COALESCE(ROUND(100.0 * COUNT(CASE WHEN status = 'success' THEN 1 END) / NULLIF(COUNT(*), 0), 1), 0)
  FROM deployment_history
  WHERE project_id = '$PROJECT_ID'
    AND created_at > NOW() - INTERVAL '7 days'
")

BUILD_SUCCESS=$(psql -t -c "
  SELECT COALESCE(ROUND(100.0 * COUNT(CASE WHEN success = true THEN 1 END) / NULLIF(COUNT(*), 0), 1), 0)
  FROM metrics_operations
  WHERE project_id = '$PROJECT_ID'
    AND tool = 'Bash'
    AND timestamp > NOW() - INTERVAL '7 days'
")

ERROR_TREND=$(psql -t -c "
  SELECT COALESCE(100 - AVG(error_rate), 100)
  FROM (
    SELECT 
      ROUND(100.0 * COUNT(CASE WHEN success = false THEN 1 END) / NULLIF(COUNT(*), 0), 1) as error_rate
    FROM metrics_operations
    WHERE project_id = '$PROJECT_ID'
      AND timestamp > NOW() - INTERVAL '7 days'
    GROUP BY DATE(timestamp)
  ) t
")

# Calculate composite score (0-100)
CONFIDENCE=$(echo "
  scale=1
  ($TEST_SUCCESS * 0.4 + $DEPLOY_SUCCESS * 0.3 + $BUILD_SUCCESS * 0.2 + $ERROR_TREND * 0.1) / 100
" | bc)

# Classify confidence level
if (( $(echo "$CONFIDENCE >= 90" | bc -l) )); then
  LEVEL="MAXIMUM"
elif (( $(echo "$CONFIDENCE >= 75" | bc -l) )); then
  LEVEL="HIGH"
elif (( $(echo "$CONFIDENCE >= 50" | bc -l) )); then
  LEVEL="MEDIUM"
elif (( $(echo "$CONFIDENCE >= 25" | bc -l) )); then
  LEVEL="LOW"
else
  LEVEL="CRITICAL"
fi

# Store confidence score
psql -q -c "
  INSERT INTO confidence_scores (
    project_id, score, level, test_success, deploy_success, timestamp
  ) VALUES (
    '$PROJECT_ID', $CONFIDENCE, '$LEVEL', $TEST_SUCCESS, $DEPLOY_SUCCESS, NOW()
  )
"

echo ""
echo "## System Confidence Score"
echo "Overall: $CONFIDENCE/100 ($LEVEL)"
echo "├─ Tests: $TEST_SUCCESS% successful"
echo "├─ Deploys: $DEPLOY_SUCCESS% successful"
echo "├─ Builds: $BUILD_SUCCESS% successful"
echo "└─ Trend: $ERROR_TREND% error-free"
echo ""
echo "At $LEVEL confidence, system auto-enforcement level: "

case "$LEVEL" in
  MAXIMUM)
    echo "✓ Fully automated, zero manual gates"
    ;;
  HIGH)
    echo "✓ Minimal manual gates, mostly automated"
    ;;
  MEDIUM)
    echo "⚠ Balanced automation and manual checks"
    ;;
  LOW)
    echo "⚠ Heavy manual oversight, limited automation"
    ;;
  CRITICAL)
    echo "✗ All operations require manual verification"
    ;;
esac
echo ""

exit 0
```

---

## Part 7: Failure Recovery & Reset Mechanisms

### Graceful Degradation

When confidence drops (after a failure), system gracefully becomes stricter.

```bash
#!/bin/bash
# failure-responder.sh - React to failures, increase oversight

INPUT=$(cat)
TOOL=$(echo "$INPUT" | jq -r '.tool_name')
ERROR=$(echo "$INPUT" | jq -r '.error // ""')

# Record failure
psql -q -c "
  INSERT INTO failure_events (tool, error_type, runner_id, timestamp)
  VALUES ('$TOOL', LEFT('$ERROR', 100), '$STRATAVORE_RUNNER_ID', NOW())
"

# Check failure rate
FAILURES_LAST_HOUR=$(psql -t -c "
  SELECT COUNT(*)
  FROM failure_events
  WHERE project_id = '$STRATAVORE_PROJECT_ID'
    AND timestamp > NOW() - INTERVAL '1 hour'
")

# Response strategy based on failure count
if (( FAILURES_LAST_HOUR >= 5 )); then
  # Critical failure rate - FULL LOCKDOWN
  psql -q -c "
    UPDATE project_state
    SET auto_enforcement = 'lockdown'
    WHERE project_id = '$STRATAVORE_PROJECT_ID'
  "
  
  echo "ALERT: High failure rate detected - System in LOCKDOWN mode" >&2
  echo "All operations now require manual verification" >&2
  echo "Run /stratavore-investigate to find root cause" >&2
  
elif (( FAILURES_LAST_HOUR >= 3 )); then
  # Elevated failure rate - ELEVATED CAUTION
  psql -q -c "
    UPDATE project_state
    SET auto_enforcement = 'caution'
    WHERE project_id = '$STRATAVORE_PROJECT_ID'
  "
  
  echo "CAUTION: Multiple failures detected - Increased verification required" >&2
  
else
  # Single failure - NORMAL with extra awareness
  echo "Failure recorded - Monitoring closely for patterns" >&2
fi

exit 0
```

### Reset Strategy

After fixing root cause, gradually restore automation.

```bash
#!/bin/bash
# recovery-monitor.sh - Monitor for recovery, restore automation

PROJECT_ID="$STRATAVORE_PROJECT_ID"

# Get current state
CURRENT_MODE=$(psql -t -c "
  SELECT auto_enforcement
  FROM project_state
  WHERE project_id = '$PROJECT_ID'
")

[ "$CURRENT_MODE" != "lockdown" ] && [ "$CURRENT_MODE" != "caution" ] && exit 0

# Check success rate since last failure
SUCCESS_SINCE_FAILURE=$(psql -t -c "
  SELECT COALESCE(ROUND(100.0 * COUNT(CASE WHEN success = true THEN 1 END) / NULLIF(COUNT(*), 0), 1), 100)
  FROM metrics_operations
  WHERE project_id = '$PROJECT_ID'
    AND timestamp > (
      SELECT COALESCE(MAX(timestamp), NOW() - INTERVAL '1 hour')
      FROM failure_events
      WHERE project_id = '$PROJECT_ID'
    )
")

# Recovery criteria
if (( $(echo "$SUCCESS_SINCE_FAILURE >= 95" | bc -l) )); then
  # System has recovered - restore normal mode
  psql -q -c "
    UPDATE project_state
    SET auto_enforcement = 'normal'
    WHERE project_id = '$PROJECT_ID'
  "
  
  echo "✓ Recovery successful: System restored to normal mode" >&2
  echo "Success rate since failure: $SUCCESS_SINCE_FAILURE%" >&2
fi

exit 0
```

---

## Part 8: Long-Term Evolution

### System Grows Smarter Over Time

**Month 1**: Learning baseline
- Hooks record what works
- Metrics accumulate
- Patterns emerge slowly

**Month 3**: Patterns stabilize
- Successful operations are highly predictable
- Error patterns are well-understood
- Automation can be confidently applied

**Month 6**: System becomes autonomous
- Most operations are fully automated
- Error prevention is proactive, not reactive
- Multi-runner coordination is seamless

**Month 12**: System optimizes itself
- Resource usage is continuously optimized
- Rare failure patterns are prevented before they manifest
- System suggests improvements based on cross-runner learnings

### Example: Year-Long Evolution

```yaml
Month 1:
  - Deployments: Manual, lots of steps, slow
  - Tests: Run sometimes, results vary
  - Confidence: 40%

Month 3:
  - Deployments: Mostly automated, quick
  - Tests: Run always, rarely fail
  - Confidence: 75%

Month 6:
  - Deployments: One-step, instant
  - Tests: Prerequisite for any change
  - Confidence: 90%

Month 12:
  - Deployments: Automatic on successful tests
  - Tests: Self-healing on failure
  - Confidence: 98%
  - System: Predicts and prevents issues before they occur
```

---

## Part 9: Building Blocks for Your System

### Required Components

Every self-reinforcing system needs:

```
1. OBSERVATION LAYER (Hooks + Metrics)
   ├─ PostToolUse: Record what happened
   ├─ PostToolUseFailure: Record failures with context
   └─ Metrics: Store data for analysis

2. DECISION LAYER (Hooks + History)
   ├─ PreToolUse: Check historical success rate
   ├─ Query past results
   └─ Adjust strictness based on history

3. FEEDBACK LAYER (Events + Context)
   ├─ Emit events when significant things happen
   ├─ Listen for events from other runners
   └─ Inject historical context at session start

4. ADAPTATION LAYER (Scoring + Adjustment)
   ├─ Calculate confidence scores
   ├─ Adjust enforcement levels
   └─ Modify hook behavior based on system state

5. RECOVERY LAYER (Monitoring + Safeguards)
   ├─ Detect failure patterns
   ├─ Gracefully degrade to manual control
   └─ Restore automation as confidence recovers
```

### Minimal Implementation

Start with these 5 hooks:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/metrics-collector.sh"
          }
        ]
      }
    ],
    "PostToolUseFailure": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/failure-recorder.sh"
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash|Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/trust-checker.sh"
          }
        ]
      }
    ],
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/confidence-scorer.sh"
          }
        ]
      }
    ]
  }
}
```

---

## Part 10: Measuring Success

### Key Metrics to Track

| Metric | What It Measures | Ideal Range |
|--------|------------------|-------------|
| Success Rate | % of operations that succeed | > 90% |
| Time to Deployment | Minutes from start to production | < 15min |
| Manual Intervention | % operations requiring human approval | < 10% |
| Error Prevention | % failures prevented before execution | > 80% |
| System Confidence | Overall system self-assessment | > 80% |
| Cross-Runner Learning | % operations from lessons of others | > 50% |

### Dashboard Query

```sql
-- Self-Reinforcement Health Dashboard
SELECT
  p.project_id,
  ROUND(100.0 * COUNT(CASE WHEN m.success THEN 1 END) / NULLIF(COUNT(*), 0), 1) as success_rate,
  ROUND(AVG(c.score), 1) as avg_confidence,
  COUNT(DISTINCT f.pattern_type) as learned_error_patterns,
  COUNT(DISTINCT e.event_type) as active_event_types,
  DATE(NOW()) as date
FROM projects p
LEFT JOIN metrics_operations m ON p.project_id = m.project_id AND m.timestamp > NOW() - INTERVAL '7 days'
LEFT JOIN confidence_scores c ON p.project_id = c.project_id AND c.timestamp > NOW() - INTERVAL '7 days'
LEFT JOIN failure_log f ON p.project_id = f.project_id
LEFT JOIN events e ON p.project_id = e.project_id AND e.created_at > NOW() - INTERVAL '7 days'
GROUP BY p.project_id
ORDER BY avg_confidence DESC;
```

---

## Conclusion: The Self-Reinforcing Vision

A truly self-reinforcing hook system has these properties:

1. **Autonomous**: Operates without human intervention in the happy path
2. **Learning**: Gets better with each interaction, remembering what works
3. **Coordinated**: Multiple agents learn from each other seamlessly
4. **Resilient**: Failures trigger learning, not chaos
5. **Optimized**: Resources are continuously optimized based on data
6. **Transparent**: Confidence scores let humans see system state
7. **Graceful**: Degradation is smooth, recovery is automatic

Over time, such a system exhibits:

- **Decreasing friction**: Deployments go from hours to seconds
- **Increasing reliability**: Success rates climb toward 99%+
- **Expanding autonomy**: Less human oversight, more automation
- **Emergent intelligence**: System discovers optimizations humans didn't anticipate

The result: **An AI development workspace that gets smarter and faster every day.**

---

## Quick Start Checklist

- [ ] Design your observation loop (metrics collection)
- [ ] Implement trust tracking (success rates)
- [ ] Create confidence scoring
- [ ] Add failure prevention rules
- [ ] Enable cross-runner learning
- [ ] Deploy confidence dashboard
- [ ] Monitor for 2 weeks (baseline)
- [ ] Measure improvement metrics
- [ ] Iterate on hook design
- [ ] Document learned patterns
- [ ] Celebrate your first "zero manual intervention" deployment
