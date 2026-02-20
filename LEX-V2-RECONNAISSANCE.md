# Lex V2 State and Configuration Architecture
## Comprehensive Reconnaissance Report

**Mission Date**: 2026-02-20
**Scout**: Pathfinder
**Target**: Migration planning for Lex V2 → Stratavore V3

---

## Executive Summary

Lex V2 is a modular Bash-based operational launcher (v2.0.0) managing Meridian Lex projects within the `~/meridian-home/` infrastructure. It uses filesystem-based state management with YAML configuration, project directories, and markdown tracking files. The architecture is hexagonal (ports and adapters), separating core logic into reusable shell modules.

**Key Findings**:
- No database backing; all state persisted as files
- Configuration centralized in `LEX-CONFIG.yaml`
- Projects stored hierarchically at `~/meridian-home/projects/`
- Session state tracked in markdown files with timestamps
- Version management via symlink/backup system
- Modular design with 6 core libraries
- Agent OS integration via dynamic function sourcing

---

## 1. File System Architecture

### 1.1 Installation Locations

```
System Installation:
  ~/.local/bin/lex                    # Active system binary (file or symlink)
  ~/.local/bin/lex.system-backup      # Stable version backup

Development:
  ~/meridian-home/projects/lex/       # Git repository (V2 source)
  ~/meridian-home/projects/lex/src/lex              # Active dev script
  ~/meridian-home/projects/lex/src/lex-version      # Version manager
```

### 1.2 Project Directory Structure

```
~/meridian-home/projects/
├── <project-name>/
│   ├── src/                         # Project source
│   ├── tests/                       # Test files
│   ├── docs/                        # Documentation
<!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
│   ├── .claude/
<!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
│   │   └── CLAUDE.md                # Project-specific Claude instructions
│   ├── README.md
│   └── .gitignore
└── setup-agentos/                   # Optional: Agent OS integration project
    ├── src/
    │   └── lex-integration.sh        # Integration functions (sourced at runtime)
    <!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
    ├── .claude/
    <!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
    │   └── CLAUDE.md
    └── README.md
```

### 1.3 Configuration Paths

```
Configuration:
  ~/meridian-home/lex-internal/config/
  ├── LEX-CONFIG.yaml                # Primary configuration (YAML)
  ├── LEX-CONFIG.yaml.bak            # Automatic backup
  └── settings.json.backup           # Legacy backup

Operational State:
  ~/meridian-home/lex-internal/state/
  ├── STATE.md                       # Current operational state
  ├── PROJECT-MAP.md                 # Project relationships
  ├── TASK-QUEUE.md                  # Task tracking
  ├── TIME-TRACKING.md               # Time sessions log
  ├── AUTONOMOUS-SESSION-REPORT.md   # Autonomous run reports
  ├── SESSION-REPORT-*.md            # Session history
  ├── SESSION-START.timestamp        # Session marker
  ├── AUTONOMOUS-MODE.lock           # Lock file for autonomous operation
  ├── time_sessions.jsonl            # Structured time tracking
  ├── github-monitor-state.json      # GitHub integration state
  └── TELEGRAM-MESSAGE-LOG.jsonl     # Message log

Logging:
  ~/meridian-home/logs/
  └── lex-invocations.log            # Action audit trail
```

---

## 2. Configuration Management

### 2.1 LEX-CONFIG.yaml Structure

**Location**: `~/meridian-home/lex-internal/config/LEX-CONFIG.yaml`

**Key Sections**:

```yaml
mode:
  current: "IDLE" | "AUTONOMOUS" | "DIRECTED" | "COLLABORATIVE"
  description: <string>

token_budget:
  daily_limit: <number>
  per_session_target: <number>
  reserved_for_commander: <number>

autonomous_mode:
  enabled: <boolean>
  max_daily_tokens: <number>
  work_hours:
    start: <time>
    end: <time>
  work_pace: <string>

scheduling:
  enabled: <boolean>
  todo_check_interval: <number>

paths:
  projects: <path>
  state: <path>
  logs: <path>
  # ... additional paths ...

claude:
  default_flags: <string>
  presets:
    <preset-name>: "<flags>"
    # e.g., "plan-mode": "--permission-mode plan"

metadata:
  vessel_id: <string>
  operator: <string>
  commissioned: <date>
  config_version: <string>
```

**Access Method**: Parsed via `yq` CLI tool by `lex-config.sh`

**Mutability**: Limited to `mode.current` via sed; other changes require manual YAML editing

---

## 3. Core State Management

### 3.1 STATE.md

**Location**: `~/meridian-home/lex-internal/state/STATE.md`

**Purpose**: Track operational state and last activity

**Content Structure**:
- Operational mode (synchronized with LEX-CONFIG.yaml)
- Timestamp of last update
- Current focus/project
- Autonomous operation status

**Update Mechanism**:
```bash
update_state() {
    local mode=$1
    local focus=$2
    local timestamp=$(date -u +"%Y-%m-%d %H:%M UTC")
    sed -i "s/^**Last Updated**:.*/\*\*Last Updated\*\*: $timestamp/" "$STATE_FILE"
}
```

### 3.2 PROJECT-MAP.md

**Location**: `~/meridian-home/lex-internal/state/PROJECT-MAP.md`

**Purpose**: Document project relationships and dependencies (read-only from lex)

**Access**: Displayed via `lex --map` or menu option

### 3.3 TASK-QUEUE.md

**Location**: `~/meridian-home/lex-internal/state/TASK-QUEUE.md`

**Purpose**: Track operational tasks

**Access**: Displayed via `lex --tasks` or menu option

### 3.4 Session Tracking

**Time Tracking**:
- `TIME-TRACKING.md` - Human-readable log
- `time_sessions.jsonl` - Structured JSON lines format with session data

**Session Reports**:
- `AUTONOMOUS-SESSION-REPORT.md` - Autonomous run summary
- `SESSION-REPORT-*.md` - Individual session reports with timestamps

**Lock Mechanism**:
- `AUTONOMOUS-MODE.lock` - Signals autonomous operation is active
- Used by mode manager to validate state consistency

---

## 4. Lex V2 Architecture

### 4.1 Core Script: `src/lex` (v2.0.0)

**Language**: Bash
**Entry Point**: Main dispatcher function
**Size**: ~235 lines + modular library sourcing

**Initialization Flow**:
```
1. set -e for error handling
2. Detect dev vs system mode
3. Source lex-lib modules:
   - core.sh (paths, utilities, color constants)
   - projects.sh (project operations)
   - config.sh (configuration display)
   - menu.sh (interactive TUI)
   - agentos.sh (Agent OS integration)
   - conversations.sh (session management)
4. check_agentos_available()
5. main() dispatcher with $@ arguments
```

**Command Categories**:

| Category | Commands |
|----------|----------|
| Help/Version | `-h`, `--help`, `-v`, `--version` |
| Projects | `-l`, `-n`, `-d`, `--new`, `--delete` |
| State | `-m`, `-s`, `-t`, `--map`, `--state`, `--tasks` |
| Config | `--config`, `--mode`, `--tokens`, `--check-auto` |
| System | `--dev-mode` |
| Conversations | `--continue`, `--resume-picker`, `--new-conversation` |
| Agent OS | `--agentos-*` (6 subcommands) |
| Projects | `<project-name>` (smart launch) |

### 4.2 Core Libraries: `lib/lex-lib/`

| Library | Responsibility | Key Functions |
|---------|-----------------|----------------|
| `core.sh` | Paths, utilities, colors | `print_*()`, `update_state()`, `log_lex_action()`, `launch_claude()` |
| `projects.sh` | Project CRUD | `list_projects()`, `select_project()`, `create_project()`, `delete_project()` |
| `config.sh` | Configuration UI | `show_config()`, `show_mode()`, `show_tokens()`, `toggle_dev_mode()` |
| `menu.sh` | Interactive menu | `show_menu()`, `show_usage()` |
| `conversations.sh` | Session management | `launch_claude_*()`, `smart_launch()`, `conversation_launch_menu()` |
| `agentos.sh` | Agent OS integration | `check_agentos_available()`, `setup_agentos_project()` |

### 4.3 Utilities and Scripts

**Core Utilities**:
- `lex-config.sh` - Configuration parser/writer (yq-based)
- `lex-mode.sh` - Operational mode sync (LEX-CONFIG ↔ STATE.md)
- `lex-budget.sh` - Token budget tracking (jq + stats cache)
- `lex-version` - Version management (symlink/backup system)

**Integration Scripts**:
- `check-autonomous-mode.sh` - Autonomous mode validator
- `lex-new` - Project creation helper
- `lex-state-check.sh` - State validation

**Supporting Tools** (in bash-scripts/):
- `test-lex.sh` - Unit tests
- `test-lex-integration.sh` - Integration tests
- `github-activity-monitor.sh` - External integrations
- `prepare-commit-msg-lex` - Git hook
- Various monitoring/rotation scripts

---

## 5. Session and Conversation Management

### 5.1 Launch Modes

**1. Direct Launch** (New session):
```bash
launch_claude <project_path> <project_name>
<!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
# Updates STATE.md, launches: exec claude [flags]
```

**2. Continue Previous**:
```bash
launch_claude_continue <project_path> <project_name>
<!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
# Launches: exec claude --continue [flags]
```

**3. Resume Picker** (Interactive selection):
```bash
launch_claude_resume_picker <project_path> <project_name>
<!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
# Launches: exec claude --resume [flags]
```

**4. Resume Specific ID**:
```bash
launch_claude_resume_id <project_path> <project_name> <session_id>
<!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
# Launches: exec claude --resume "<session_id>" [flags]
```

**5. New Conversation** (Explicit):
```bash
launch_claude_new <project_path> <project_name>
<!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
# Launches: exec claude [flags] (no --continue/--resume)
```

### 5.2 Smart Launch Logic

```bash
smart_launch() {
    if [ -d "$project_path/.claude" ]; then
        # Project has history → show menu
        conversation_launch_menu "$project_path" "$project_name"
    else
        # New project → launch directly
        launch_claude "$project_path" "$project_name"
    fi
}
```

### 5.3 Meridian Lex Integration Points

**Flags System**:
- Source: `LEX_CLAUDE_FLAGS` env var or config defaults
- Can be: preset, preset + --continue, --dangerously-skip-permissions, etc.
- Passed directly to `claude` binary via `exec`

**Session Persistence**:
- Lex delegates to Meridian Lex's native `--continue`, `--resume` flags
- No direct session state management by Lex
<!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
- `.claude/CLAUDE.md` provides project-specific instructions to Claude

---

## 6. Version Management System

### 6.1 Version States

**System State Detection**:
```bash
check_version() {
    if [ -L "$SYSTEM_LEX" ]; then
        # Symlink → dev mode (fast iteration)
        echo "dev"
    elif [ -f "$SYSTEM_LEX" ]; then
        # Regular file → system mode (stable)
        echo "system"
    else
        echo "none"
    fi
}
```

### 6.2 Version Switching

| Operation | Action | Result |
|-----------|--------|--------|
| `dev` | Backup system, create symlink to src/lex | Immediate changes active |
| `system` | Remove symlink, restore from backup | Use stable version |
| `install` | Copy src/lex to system location | Promote dev to stable |

**Paths**:
- System: `~/.local/bin/lex`
- Dev: `~/meridian-home/projects/lex/src/lex`
- Backup: `~/.local/bin/lex.system-backup`

---

## 7. Agent OS Integration

### 7.1 Dynamic Loading

**Detection**:
```bash
check_agentos_available() {
    if [ -f "$SETUP_AGENTOS_DIR/src/lex-integration.sh" ]; then
        source "$SETUP_AGENTOS_DIR/src/lex-integration.sh"
        export AGENTOS_AVAILABLE=true
    else
        export AGENTOS_AVAILABLE=false
    fi
}
```

**Runtime Check**:
- All `--agentos-*` commands verify `AGENTOS_AVAILABLE` flag
- If false, print helpful error and suggest `--agentos-setup`

### 7.2 Integration Functions

**Required Exports** from `lex-integration.sh`:
```bash
agentos_init_current_project(profile)
agentos_status(target_path)
agentos_verify(target_path)
agentos_install_base()
agentos_update_base()
```

**Setup Assistance** (v1.2+):
- If unavailable, `--agentos-setup` creates `setup-agentos` project
- Generates placeholder functions for user implementation
- Enables iterative development of Agent OS integration

---

## 8. Interactive Menu System

### 8.1 TUI Structure

**Menu Sections**:
```
Context:        [0] Global, [1] Select Project, [f] Full Access, [2] New, [3] Delete
Information:    [4] Map, [5] State, [6] Tasks
Configuration:  [7] Config, [8] Mode, [9] Budget
System:         [d] Dev Mode, [a] Agent OS Setup (if unavailable)
Action:         [q] Exit
```

**Navigation**:
- Recursive menu system; invalid choices loop with error
- Each selection transitions to new context or returns to menu
<!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
- Exit via 'q' or exec claude (replaces process)

---

## 9. Action Logging and Auditing

### 9.1 Lex Invocation Log

**Location**: `~/meridian-home/logs/lex-invocations.log`

**Entry Format**:
```
[YYYY-MM-DD HH:MM:SS UTC] ACTION | details
```

**Logged Actions**:
- LAUNCH (new conversation)
- CONTINUE (resume most recent)
- RESUME_PICKER (open selector)
- RESUME_ID (resume specific session)
- NEW_CONVERSATION (explicit new)

**Example**:
```
[2026-02-20 14:32:15 UTC] LAUNCH | Stratavore (new conversation) | flags= | pwd=/home/meridian/meridian-home/projects/Stratavore
```

---

## 10. Dependencies and External Integration

### 10.1 Required Tools

| Tool | Version | Usage |
|------|---------|-------|
| Bash | 4.0+ | Script engine (uses arrays) |
| Git | Any | Project initialization |
| Meridian Lex | CLI | Session launcher |
| sed | GNU | In-place file editing |
| readlink | coreutils | Symlink resolution |
| yq | Any | YAML parsing |
| jq | Any | JSON parsing (budget) |

### 10.2 External Files

| File | Source | Usage |
|------|--------|-------|
<!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
| `~/.claude/stats-cache.json` | Claude Code | Token usage stats |
| `LEX-CONFIG.yaml` | Manually maintained | Global configuration |
| `STATE.md` | Lex updates | Operational state |

### 10.3 Environment Variables

| Variable | Set By | Usage |
|----------|--------|-------|
| `LEX_DEV_MODE` | src/lex | true if running from projects/lex/ |
| `LEX_LIB_DIR` | src/lex | Points to lib/lex-lib/ |
| `LEX_VERSION` | src/lex | Currently "2.0.0" |
| `LEX_CLAUDE_FLAGS` | Main or CLI args | Passed to claude binary |
| `LEX_CURRENT_PROJECT` | Various | Tracks active project context |
| `AGENTOS_AVAILABLE` | agentos.sh | Boolean: Agent OS integration available |

---

## 11. Data Persistence Model

### 11.1 Persistence Layer

**Primary Storage**:
- Filesystem (directories, markdown files, YAML)
- No database or structured store
- Git-tracked where appropriate

**State Categories**:

| Category | Storage | Format | Mutability |
|----------|---------|--------|-----------|
| Projects | `~/meridian-home/projects/` | Directories | Direct filesystem |
| Config | `LEX-CONFIG.yaml` | YAML | yq/sed edits |
| Operational State | `STATE.md` | Markdown | sed in-place updates |
| Tasks | `TASK-QUEUE.md` | Markdown | Manual editing |
<!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
| Sessions | `.claude/` dirs | Files + history | Claude Code native |
| Time Tracking | `.jsonl` files | JSON lines | Append-only |
| Logs | `.log` files | Text | Append-only |

### 11.2 Synchronization Points

**Mode Sync** (lex-mode.sh):
```
User Input → LEX-CONFIG.yaml ↔ STATE.md
                 (set operation)
```

**State Broadcasts**:
- STATE.md timestamp updated on every lex launch
- No real-time synchronization
- Last-write-wins for conflicting updates

---

## 12. Key Design Patterns

### 12.1 Hexagonal Architecture

**Ports** (External interfaces):
- Meridian Lex launcher
- Filesystem I/O (projects, state, config)
- Configuration system (yq)

**Adapters** (Implementation):
- Individual modules in lib/lex-lib/
- Each module exports specific functions
- Core module provides utilities to others

### 12.2 Module Loading Pattern

```bash
# Core initialization
source "$LEX_LIB_DIR/core.sh"        # Must be first

# Dependent modules
source "$LEX_LIB_DIR/projects.sh"    # Uses core exports
source "$LEX_LIB_DIR/config.sh"      # Uses core exports
# ... etc ...
```

### 12.3 Safe Defaults

- **Error handling**: `set -e` (exit on error)
- **Quoting**: All variables quoted: `"$var"`
- **File checks**: Always test existence before use
- **Backups**: Version manager backs up before destructive ops
- **Confirmation**: Delete operations require name confirmation

---

## 13. Version History (V2.0.0)

**Major Changes from V1.x**:
- Modular architecture (6 separate libraries)
- Flag system for Meridian Lex integration
- Preset system via LEX-CONFIG.yaml
- Conversation management (continue/resume/picker)
- Smart launch (auto-menu detection)
- Configuration subsystem
- Task queue tracking
- Dev mode toggle accessible from both versions

**Recent Additions**:
- V1.2 (2026-02-06): Agent OS setup assistance
- V1.1 (2026-02-06): Full Agent OS integration
- V1.0 (2026-02-05): Initial release

---

## 14. Migration Implications for Stratavore

### 14.1 State to Migrate

**Must Migrate**:
1. Project registry (list of projects at ~/meridian-home/projects/)
2. Operational mode (IDLE/AUTONOMOUS/DIRECTED/COLLABORATIVE)
3. Configuration values (token budgets, presets, paths)
4. Session history (if tracking beyond Lex's native storage)
5. Time tracking data (time_sessions.jsonl)

**Can Derive**:
1. Project relationships (from PROJECT-MAP.md)
2. Task queue (from TASK-QUEUE.md)
3. Recent activity logs (from lex-invocations.log)

### 14.2 State Not Directly Comparable

- Meridian Lex manages conversation storage (outside Lex scope)
<!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
- Lex only references `.claude/CLAUDE.md` for instructions
<!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
- Actual session files stored by Claude Code at `~/.claude/`

### 14.3 Database Schema Candidates

**PostgreSQL Schema** (for Stratavore):
```sql
-- Projects
CREATE TABLE projects (
    id UUID PRIMARY KEY,
    name VARCHAR UNIQUE NOT NULL,
    path VARCHAR NOT NULL,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

-- Operational State
CREATE TABLE state (
    key VARCHAR PRIMARY KEY,
    value TEXT,
    updated_at TIMESTAMP
);

-- Configuration
CREATE TABLE config (
    key VARCHAR PRIMARY KEY,
    value JSONB,
    updated_at TIMESTAMP
);

-- Sessions
CREATE TABLE sessions (
    id UUID PRIMARY KEY,
    project_id UUID REFERENCES projects,
    launched_at TIMESTAMP,
    mode VARCHAR,
    flags TEXT
);

-- Time Tracking
CREATE TABLE time_sessions (
    id UUID PRIMARY KEY,
    project_id UUID REFERENCES projects,
    start_time TIMESTAMP,
    end_time TIMESTAMP,
    duration INTERVAL
);
```

### 14.4 CLI Compatibility

**Preserve**:
- `lex <project-name>` → direct launch
- `lex -l` / `--list` → list projects
- `lex -n <name>` / `--new` → create project
- `lex --mode` → get/set mode
- `lex --config` → show config

**Transform**:
- `lex --tokens` → query token budget from PostgreSQL
- `lex -m` / `--map` → derive from project relationships in DB
- `lex -s` / `--state` → query state table
- `lex --agentos-*` → adapt to new Agent OS integration

---

## 15. Critical Files for Migration

### 15.1 Source Files (must preserve logic)

| File | Lines | Purpose |
|------|-------|---------|
| src/lex | 235 | Main dispatcher |
| lib/lex-lib/core.sh | 85 | Utilities |
| lib/lex-lib/projects.sh | 98 | Project operations |
| lib/lex-lib/conversations.sh | 127 | Session management |
| lib/lex-lib/menu.sh | 126 | Interactive menu |
| lib/lex-lib/config.sh | 110 | Config display |
| lib/lex-lib/agentos.sh | 136 | Agent OS integration |

### 15.2 Configuration Files (must migrate data)

| File | Format | Priority |
|------|--------|----------|
| LEX-CONFIG.yaml | YAML | **HIGH** |
| STATE.md | Markdown | **HIGH** |
| time_sessions.jsonl | JSON Lines | **MEDIUM** |
| TASK-QUEUE.md | Markdown | **MEDIUM** |
| PROJECT-MAP.md | Markdown | **LOW** |

---

## 16. Summary for Stratavore Integration

### Key Takeaways

1. **Current State**: Lex V2 is a mature bash-based orchestrator with modular design, no database backing, and filesystem-centric persistence.

2. **Migration Scope**: Need to replicate state management (projects, config, mode, time tracking) using PostgreSQL + pgvector in Stratavore, preserving CLI ergonomics.

3. **Data Volume**: Low (project count ~10-20, config ~100 keys, time entries ~100-1000s), ideal for structured DB migration.

4. **Integration Points**:
- Meridian Lex interaction: Direct launcher, no state management needed
   - Agent OS: Dynamic sourcing, recommend service/plugin pattern in Go
   - Configuration: YAML → PostgreSQL, CLI tool via Cobra/Viper

5. **Testing Strategy**:
   - Unit tests for state migrations
   - CLI parity tests (same flags, same UX)
- Integration tests with Meridian Lex
   - Agent OS compatibility matrix

---

## Appendix: File Tree

```
~/meridian-home/
├── projects/
│   └── lex/                                 [Git repo, V2 source]
│       ├── src/
│       │   ├── lex                          [Main script]
│       │   ├── lex-version                  [Version mgr]
│       │   └── lex-v1.3-backup
│       ├── lib/
│       │   └── lex-lib/
│       │       ├── core.sh
│       │       ├── projects.sh
│       │       ├── config.sh
│       │       ├── menu.sh
│       │       ├── conversations.sh
│       │       ├── agentos.sh
│       │       ├── lex-config.sh
│       │       ├── lex-mode.sh
│       │       └── lex-budget.sh
<!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
│       ├── .claude/
<!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
│       │   └── CLAUDE.md                    [Architecture docs]
│       ├── CHANGELOG.md
│       └── README.md
│
├── lex-internal/
│   ├── config/
│   │   ├── LEX-CONFIG.yaml                  [Primary config]
│   │   └── LEX-CONFIG.yaml.bak
│   └── state/
│       ├── STATE.md                         [Current state]
│       ├── PROJECT-MAP.md
│       ├── TASK-QUEUE.md
│       ├── TIME-TRACKING.md
│       ├── AUTONOMOUS-SESSION-REPORT.md
│       ├── SESSION-REPORT-*.md
│       ├── time_sessions.jsonl
│       ├── AUTONOMOUS-MODE.lock
│       └── github-monitor-state.json
│
├── bash-scripts/                            [Utilities]
│   ├── lex-config.sh
│   ├── lex-mode.sh
│   ├── lex-budget.sh
│   ├── check-autonomous-mode.sh
│   └── [other monitoring/integration scripts]
│
└── logs/
    └── lex-invocations.log

System Locations:
  ~/.local/bin/lex                           [Active binary]
  ~/.local/bin/lex.system-backup             [Backup]
  <!-- IDENTITY-EXCEPTION: functional internal reference — not for public exposure -->
  ~/.claude/stats-cache.json                 [Claude Code stats]
```

---

**Report Complete**
**Ready for Stratavore Migration Planning**

