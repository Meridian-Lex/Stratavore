# Sprint System Patterns

## State Machine

Sprints: `pending → running → completed / failed / cancelled`
Tasks: `pending → running → completed / failed / skipped`

- Never skip states — always transition in order
- `skipped` only when upstream `depends_on` task failed

## Task Sequencing

Tasks execute in `sequence_number` order. Dependencies via `depends_on` (task ID).

```sql
-- Tasks with same sequence_number can run in parallel
-- Tasks with depends_on wait for that task to complete
sequence_number INTEGER NOT NULL
depends_on      UUID REFERENCES sprint_tasks(id)
```

## Model Registry

Models stored in `model_registry` table — never hardcoded. New models require a DB row, not a code change.

Required fields per model:
- `name` — unique identifier used in task config
- `backend` — `messages-api` | `ollama` | `openrouter`
- `tier` — `lex` | `haiku45` | `haiku3` | `ollama` | `custom`
- `cost_per_million_input/output` — for budget tracking

## Execution Audit

Every sprint run creates a `sprint_executions` record:
- `tasks_total`, `tasks_completed`, `tasks_failed`
- `total_tokens_input`, `total_tokens_output`
- `total_cost_usd`, `duration_ms`

Always write execution record even on partial failure — audit trail must be complete.

## Prompt Design

Sprint tasks carry `system_prompt` + `user_prompt` separately.
`system_prompt` sets persona/context; `user_prompt` is the specific task instruction.
Never combine them into a single field.
