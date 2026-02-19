# The Ralph Playbook

A comprehensive guide to running autonomous AI coding loops.

Thought up by Geoff Huntley · Regurgitated by Clayton Farr  

Original source - https://ghuntley.com/ralph

---

## TL;DR

Ralph is an autonomous coding methodology that runs Claude in a continuous loop, using file-based state to maintain context across iterations.  

Each loop:  
**read plan → pick task → implement → test → commit → clear context → repeat**

**Why it works:** Fresh context each iteration keeps the AI in its "smart zone." File-based memory (specs, plan, agents file) persists learnings. Backpressure (utilities, tests, builds) forces self-correction.

**Key files:**  
`PROMPT.md` (instructions) + `AGENTS.md` (operational guide) + `IMPLEMENTATION_PLAN.md` (task list) + `specs/*` (requirements)

---

December 2025 boiled Ralph's powerful yet dumb little face to the top of most AI-related timelines.

I try to pay attention to the crazy-smart insights @GeoffreyHuntley shares, but I can't say Ralph really clicked for me this summer. Now, all of the recent hubbub has made it hard to ignore.

@mattpocockuk and @ryancarson's overviews helped a lot – right until Geoff came in and said "nah".

> Geoff Huntley responding "nah" to various Ralph interpretations

So what is the optimal way to Ralph?

Many folks seem to be getting good results with various shapes – but I wanted to read the tea leaves as closely as possible from the person who not only captured this approach but also has had the most ass-time in the seat putting it through its paces.

So I dug in to really RTFM on recent videos and Geoff's original post to try and untangle for myself what works best.

Below is the result – a (likely OCD-fueled) Ralph Playbook that organizes the miscellaneous details for putting this all into practice w/o hopefully neutering it in the process.

---

## Note

Digging into all of this has also brought to mind some possibly valuable additional enhancements to the core approach that aim to stay aligned with the guidelines that make Ralph work so well.

Hope this helps you out - Clayton

---

# Workflow

A picture is worth a thousand tweets and an hour-long video. Geoff's overview here (sign up to his newsletter to see full article) really helped clarify the workflow details for moving from:

1) Idea  
2) Individual JTBD-aligned specs  
3) Comprehensive implementation plan  
4) Ralph work loops  

> Ralph workflow diagram showing the three phases  
> The Ralph Process: From idea to implementation through specs, planning, and building loops

---

## 🗘 Three Phases, Two Prompts, One Loop

### Phase 1: Define Requirements (LLM conversation)

- Discuss project ideas → identify Jobs to Be Done (JTBD)  
- Break individual JTBD into topic(s) of concern  
- Use subagents to load info from URLs into context  
- LLM understands JTBD topic of concern: subagent writes `specs/FILENAME.md` for each topic  

---

## Phase 2 / 3: Run Ralph Loop

Same loop mechanism, different prompts for different objectives:

| Mode      | When to use                              | Prompt focus |
|-----------|-------------------------------------------|--------------|
| PLANNING  | No plan exists, or plan is stale/wrong    | Generate/update `IMPLEMENTATION_PLAN.md` only |
| BUILDING  | Plan exists                               | Implement from plan, commit, update plan as side effect |

### Prompt differences per mode

- **PLANNING prompt** does gap analysis (specs vs code) and outputs a prioritized TODO list – no implementation, no commits.  
- **BUILDING prompt** assumes plan exists, picks tasks from it, implements, runs tests (backpressure), commits.

### Why use the loop for both modes?

- BUILDING requires it: inherently iterative (many tasks × fresh context = isolation)  
- PLANNING uses it for consistency: same execution model, though often completes in 1–2 iterations  
- Flexibility: if plan needs refinement, loop allows multiple passes reading its own output  
- Simplicity: one mechanism for everything; clean file I/O; easy stop/restart  

Context loaded each iteration: `PROMPT.md` + `AGENTS.md`

---

## PLANNING mode loop lifecycle

- Subagents study `specs/*` and existing `/src`  
- Compare specs against code (gap analysis)  
- Create/update `IMPLEMENTATION_PLAN.md` with prioritized tasks  
- No implementation  

---

## BUILDING mode loop lifecycle

- **Orient** – subagents study `specs/*` (requirements)  
- **Read plan** – study `IMPLEMENTATION_PLAN.md`  
- **Select** – pick the most important task  
- **Investigate** – subagents study relevant `/src` ("don't assume not implemented")  
- **Implement** – N subagents for file operations  
- **Validate** – 1 subagent for build/tests (backpressure)  
- Update `IMPLEMENTATION_PLAN.md` – mark task done, note discoveries/bugs  
- Update `AGENTS.md` – if operational learnings  
- Commit  
- Loop ends → context cleared → next iteration starts fresh  

---

# Concepts

| Term | Definition |
|------|------------|
| Job to be Done (JTBD) | High-level user need or outcome |
| Topic of Concern | A distinct aspect/component within a JTBD |
| Spec | Requirements doc for one topic of concern (`specs/FILENAME.md`) |
| Task | Unit of work derived from comparing specs to code |

### Relationships

- 1 JTBD → multiple topics of concern  
- 1 topic of concern → 1 spec  
- 1 spec → multiple tasks (specs are larger than tasks)

### Example: JTBD breakdown

- JTBD: "Help designers create mood boards"  
- Topics: image collection, color extraction, layout, sharing  
- Each topic → one spec file  
- Each spec → many tasks in implementation plan  

---

## Topic Scope Test

**"One Sentence Without 'And'"**

Can you describe the topic of concern in one sentence without conjoining unrelated capabilities?

- ✓ "The color extraction system analyzes images to identify dominant colors"  
- ✗ "The user system handles authentication, profiles, and billing" → 3 topics  

If you need "and" to describe what it does, it's probably multiple topics.

---

# Key Principles

Four principles drive Ralph's effectiveness:

- Constrained context  
- Backpressure  
- Autonomous action  
- Human oversight over the loop — not in it  

---

## ⏳ Context Is Everything

- When 200K+ tokens advertised = ~176K truly usable  
- 40–60% context utilization for "smart zone"  
- Tight tasks + 1 task per loop = 100% smart zone context utilization  

This informs and drives everything else:

- Use the main agent/context as a scheduler  
- Use subagents as memory extension (~156kb each, garbage collected)  
- Simplicity and brevity win  
- Prefer Markdown over JSON  

---

## 🧭 Steering Ralph: Patterns + Backpressure

### Steer upstream

- Ensure deterministic setup:
  - Allocate first ~5,000 tokens for specs  
  - Same files loaded each iteration (`PROMPT.md` + `AGENTS.md`)  
- Existing code shapes output  
- Add/update utilities to steer patterns  

### Steer downstream

- Create backpressure to reject invalid work  
- Wire in tests, typechecks, lints, builds  
- Prompt says "run tests"; `AGENTS.md` defines actual commands  
- LLM-as-judge possible for subjective acceptance criteria  

Remind Ralph in `Prompt.md`:

> "Important: When authoring documentation, capture the why - tests and implementation importance."

---

## 🙏 Let Ralph Ralph

- Lean into self-identify, self-correct, self-improve  
- Applies to plan, task definition, prioritization  
- Eventual consistency via iteration  

### Use Protection (Really)

Ralph requires:

```
--dangerously-skip-permissions
```

- Bypasses Claude's permission system  
- Sandbox becomes the only security boundary  
- Limit credentials and network access  
- Docker sandboxes (local), Fly Sprites/E2B/etc. (remote)  

Escape hatches:

- `Ctrl+C`  
- `git reset --hard`  
- Regenerate plan  

---

# 🚦 Move Outside the Loop

Your job is to sit **on** the loop, not **in** it.

Observe and course correct. Prompts evolve through failure patterns.

Tune reactively. Add signs when Ralph fails.

Signs can be:

- Prompt guardrails  
- `AGENTS.md` updates  
- Utilities in codebase  
- Other discoverable inputs  

Tip:

- Start with empty `AGENTS.md`  
- Spot-test  
- Watch early loops  
- Tune only as needed  

### The plan is disposable

Regenerate when:

- Off track  
- Stale  
- Cluttered  
- Specs changed  
- Confusion about state  

---

# Loop Mechanics

## 🔄 Task Selection

Geoff's minimal `loop.sh`:

```bash
while :; do cat PROMPT.md | claude ; done
```

Continuation mechanism:

1. Bash feeds prompt  
2. Agent studies `IMPLEMENTATION_PLAN.md`  
3. Completes one task  
4. Commits and exits  
5. Loop restarts with fresh context  

Shared state = `IMPLEMENTATION_PLAN.md`.

---

## Task Execution

Control relies on:

- Scope discipline  
- Backpressure  
- Natural completion  

Ralph may fail. Add guardrails. Iterate.

---

# 📁 Files

```
project-root/
├── loop.sh
├── PROMPT_build.md
├── PROMPT_plan.md
├── AGENTS.md
├── IMPLEMENTATION_PLAN.md
├── specs/
│   ├── [jtbd-topic-a].md
│   └── [jtbd-topic-b].md
├── src/
└── src/lib/
```

| File | Purpose | Modified By |
|------|----------|-------------|
| loop.sh | Task loop orchestration | You |
| PROMPT_*.md | Mode instructions | You |
| AGENTS.md | Operational guide | Ralph + You |
| IMPLEMENTATION_PLAN.md | Task list | Ralph |
| specs/* | Requirements | You + Ralph |

---

## AGENTS.md

- Canonical operational guide  
- Not a changelog  
- ~60 lines  
- Defines build/test commands  

---

## IMPLEMENTATION_PLAN.md

- Prioritized task list  
- Generated in PLANNING  
- Updated in BUILDING  
- Regenerable  

---

## specs/*

- One file per topic  
- Source of truth  
- Used by both modes  

---

## src/ and src/lib/

Application source and shared utilities.

## Enhancements?

I'm still determining the value/viability of these, but the opportunities sound promising:

- [Claude's AskUserQuestionTool for Planning](#use-claudes-askuserquestiontool-for-planning) - use Claude's built-in interview tool to systematically clarify JTBD, edge cases, and acceptance criteria for specs.
- [Acceptance-Driven Backpressure](#acceptance-driven-backpressure) - Derive test requirements during planning from acceptance criteria. Prevents "cheating" - can't claim done without appropriate tests passing.
- [Non-Deterministic Backpressure](#non-deterministic-backpressure) - Using LLM-as-judge for tests against subjective tasks (tone, aesthetics, UX). Binary pass/fail reviews that iterate until pass.
- [Ralph-Friendly Work Branches](#ralph-friendly-work-branches) - Asking Ralph to "filter to feature X" at runtime is unreliable. Instead, create scoped plan per branch upfront.
- [JTBD → Story Map → SLC Release](#jtbd--story-map--slc-release) - Push the power of "Letting Ralph Ralph" to connect JTBD's audience and activities to Simple/Lovable/Complete releases.

---

### Use Claude's AskUserQuestionTool for Planning

During Phase 1 (Define Requirements), use Claude's built-in `AskUserQuestionTool` to systematically explore JTBD, topics of concern, edge cases, and acceptance criteria through structured interview before writing specs.

_When to use:_ Minimal/vague initial requirements, need to clarify constraints, or multiple valid approaches exist.

_Invoke:_ "Interview me using AskUserQuestion to understand [JTBD/topic/acceptance criteria/...]"

Claude will ask targeted questions to clarify requirements and ensure alignment before producing `specs/*.md` files.

_Flow:_

1. Start with known information →
2. _Claude interviews via AskUserQuestion_ →
3. Iterate until clear →
4. Claude writes specs with acceptance criteria →
5. Proceed to planning/building

No code or prompt changes needed - this simply enhances Phase 1 using existing Claude Code capabilities.

_Inspiration_ - [Thariq's X post](https://x.com/trq212/status/2005315275026260309):

---

### Acceptance-Driven Backpressure

Geoff's Ralph _implicitly_ connects specs → implementation → tests through emergent iteration. This enhancement would make that connection _explicit_ by deriving test requirements during planning, creating a direct line from "what success looks like" to "what verifies it."

This enhancement connects acceptance criteria (in specs) directly to test requirements (in implementation plan), improving backpressure quality by:

- _Preventing "no cheating"_ - Can't claim done without required tests derived from acceptance criteria
- _Enabling TDD workflow_ - Test requirements known before implementation starts
- _Improving convergence_ - Clear completion signal (required tests pass) vs ambiguous ("seems done?")
- _Maintaining determinism_ - Test requirements in plan (known state) not emergent (probabilistic)

#### Compatibility with Core Philosophy

| Principle             | Maintained? | How                                                         |
| --------------------- | ----------- | ----------------------------------------------------------- |
| Monolithic operation  | ✅ Yes      | One agent, one task, one loop at a time                     |
| Backpressure critical | ✅ Yes      | Tests are the mechanism, just derived explicitly now        |
| Context efficiency    | ✅ Yes      | Planning decides tests once vs building rediscovering       |
| Deterministic setup   | ✅ Yes      | Test requirements in plan (known state) not emergent        |
| Let Ralph Ralph       | ✅ Yes      | Ralph still prioritizes and chooses implementation approach |
| Plan is disposable    | ✅ Yes      | Wrong test requirements? Regenerate plan                    |
| "Capture the why"     | ✅ Yes      | Test intent documented in plan before implementation        |
| No cheating           | ✅ Yes      | Required tests prevent placeholder implementations          |

#### The Prescriptiveness Balance

The critical distinction:

_Acceptance criteria_ (in specs) = Behavioral outcomes, observable results, what success looks like

- ✅ "Extracts 5-10 dominant colors from any uploaded image"
- ✅ "Processes images <5MB in <100ms"
- ✅ "Handles edge cases: grayscale, single-color, transparent backgrounds"

_Test requirements_ (in implementation plan) = Verification points derived from acceptance criteria

- ✅ "Required tests: Extract 5-10 colors, Performance <100ms, Handle grayscale edge case"

_Implementation approach_ (up to Ralph) = Technical decisions about how to achieve it

- ❌ "Use K-means clustering with 3 iterations and LAB color space conversion"

The key: _Specify WHAT to verify (outcomes), not HOW to implement (approach)_

This maintains "Let Ralph Ralph" principle - Ralph decides implementation details while having clear success signals.

#### Architecture: Three-Phase Connection

```
Phase 1: Requirements Definition
    specs/*.md + Acceptance Criteria
    ↓
Phase 2: Planning (derives test requirements)
    IMPLEMENTATION_PLAN.md + Required Tests
    ↓
Phase 3: Building (implements with tests)
    Implementation + Tests → Backpressure
```

#### Phase 1: Requirements Definition

During the human + LLM conversation that produces specs:

- Discuss JTBD and break into topics of concern
- Use subagents to load external context as needed
- _Discuss and define acceptance criteria_ - what observable, verifiable outcomes indicate success
- Keep criteria behavioral (outcomes), not implementation (how to build it)
- LLM writes specs including acceptance criteria however makes most sense for the spec
- Acceptance criteria become the foundation for deriving test requirements in planning phase

#### Phase 2: Planning Mode Enhancement

Modify `PROMPT_plan.md` instruction 1 to include test derivation. Add after the first sentence:

```markdown
For each task in the plan, derive required tests from acceptance criteria in specs - what specific outcomes need verification (behavior, performance, edge cases). Tests verify WHAT works, not HOW it's implemented. Include as part of task definition.
```

#### Phase 3: Building Mode Enhancement

Modify `PROMPT_build.md` instructions:

_Instruction 1:_ Add after "choose the most important item to address":

```markdown
Tasks include required tests - implement tests as part of task scope.
```

_Instruction 2:_ Replace "run the tests for that unit of code" with:

```markdown
run all required tests specified in the task definition. All required tests must exist and pass before the task is considered complete.
```

_Prepend new guardrail_ (in the 9s sequence):

```markdown
999. Required tests derived from acceptance criteria must exist and pass before committing. Tests are part of implementation scope, not optional. Test-driven development approach: tests can be written first or alongside implementation.
```

---

### Non-Deterministic Backpressure

Some acceptance criteria resist programmatic validation:

- _Creative quality_ - Writing tone, narrative flow, engagement
- _Aesthetic judgments_ - Visual harmony, design balance, brand consistency
- _UX quality_ - Intuitive navigation, clear information hierarchy
- _Content appropriateness_ - Context-aware messaging, audience fit

These require human-like judgment but need backpressure to meet acceptance criteria during building loop.

_Solution:_ Add LLM-as-Judge tests as backpressure with binary pass/fail.

LLM reviews are non-deterministic (same artifact may receive different judgments across runs). This aligns with Ralph philosophy: "deterministically bad in an undeterministic world." The loop provides eventual consistency through iteration—reviews run until pass, accepting natural variance.

#### What Needs to Be Created (First Step)

Create two files in `src/lib/`:

```
src/lib/
  llm-review.ts          # Core fixture - single function, clean API
  llm-review.test.ts     # Reference examples showing the pattern (Ralph learns from these)
```

##### `llm-review.ts` - Binary pass/fail API Ralph discovers:

```typescript
interface ReviewResult {
  pass: boolean;
  feedback?: string; // Only present when pass=false
}

function createReview(config: {
  criteria: string; // What to evaluate (behavioral, observable)
  artifact: string; // Text content OR screenshot path
  intelligence?: "fast" | "smart"; // Optional, defaults to 'fast'
}): Promise<ReviewResult>;
```

_Multimodal support:_ Both intelligence levels would use multimodal model (text + vision). Artifact type detection is automatic:

- Text evaluation: `artifact: "Your content here"` → Routes as text input
- Vision evaluation: `artifact: "./tmp/screenshot.png"` → Routes as vision input (detects .png, .jpg, .jpeg extensions)

_Intelligence levels_ (quality of judgment, not capability type):

- `fast` (default): Quick, cost-effective models for straightforward evaluations
  - Example: Gemini 3.0 Flash (multimodal, fast, cheap)
- `smart`: Higher-quality models for nuanced aesthetic/creative judgment
  - Example: GPT 5.1 (multimodal, better judgment, higher cost)

The fixture implementation selects appropriate models. (Examples are current options, not requirements.)

##### `llm-review.test.ts` - Shows Ralph how to use it (text and vision examples):

```typescript
import { createReview } from "@/lib/llm-review";

// Example 1: Text evaluation
test("welcome message tone", async () => {
  const message = generateWelcomeMessage();
  const result = await createReview({
    criteria:
      "Message uses warm, conversational tone appropriate for design professionals while clearly conveying value proposition",
    artifact: message, // Text content
  });
  expect(result.pass).toBe(true);
});

// Example 2: Vision evaluation (screenshot path)
test("dashboard visual hierarchy", async () => {
  await page.screenshot({ path: "./tmp/dashboard.png" });
  const result = await createReview({
    criteria:
      "Layout demonstrates clear visual hierarchy with obvious primary action",
    artifact: "./tmp/dashboard.png", // Screenshot path
  });
  expect(result.pass).toBe(true);
});

// Example 3: Smart intelligence for complex judgment
test("brand visual consistency", async () => {
  await page.screenshot({ path: "./tmp/homepage.png" });
  const result = await createReview({
    criteria:
      "Visual design maintains professional brand identity suitable for financial services while avoiding corporate sterility",
    artifact: "./tmp/homepage.png",
    intelligence: "smart", // Complex aesthetic judgment
  });
  expect(result.pass).toBe(true);
});
```

_Ralph learns from these examples:_ Both text and screenshots work as artifacts. Choose based on what needs evaluation. The fixture handles the rest internally.

_Future extensibility:_ Current design uses single `artifact: string` for simplicity. Can expand to `artifact: string | string[]` if clear patterns emerge requiring multiple artifacts (before/after comparisons, consistency across items, multi-perspective evaluation). Composite screenshots or concatenated text could handle most multi-item needs.

#### Integration with Ralph Workflow

_Planning Phase_ - Update `PROMPT_plan.md`:

After:

```
...Study @IMPLEMENTATION_PLAN.md to determine starting point for research and keep it up to date with items considered complete/incomplete using subagents.
```

Insert this:

```
When deriving test requirements from acceptance criteria, identify whether verification requires programmatic validation (measurable, inspectable) or human-like judgment (perceptual quality, tone, aesthetics). Both types are equally valid backpressure mechanisms. For subjective criteria that resist programmatic validation, explore src/lib for non-deterministic evaluation patterns.
```

_Building Phase_ - Update `PROMPT_build.md`:

Prepend new guardrail (in the 9s sequence):

```markdown
9999. Create tests to verify implementation meets acceptance criteria and include both conventional tests (behavior, performance, correctness) and perceptual quality tests (for subjective criteria, see src/lib patterns).
```

_Discovery, not documentation:_ Ralph learns LLM review patterns from `llm-review.test.ts` examples during `src/lib` exploration (Phase 0c). No AGENTS.md updates needed - the code examples are the documentation.

#### Compatibility with Core Philosophy

| Principle             | Maintained? | How                                                                                                                                          |
| --------------------- | ----------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| Backpressure critical | ✅ Yes      | Extends backpressure to non-programmatic acceptance                                                                                          |
| Deterministic setup   | ⚠️ Partial  | Criteria in plan (deterministic), evaluation non-deterministic but converges through iteration. Intentional tradeoff for subjective quality. |
| Context efficiency    | ✅ Yes      | Fixture reused via `src/lib`, small test definitions                                                                                         |
| Let Ralph Ralph       | ✅ Yes      | Ralph discovers pattern, chooses when to use, writes criteria                                                                                |
| Plan is disposable    | ✅ Yes      | Review requirements part of plan, regenerate if wrong                                                                                        |
| Simplicity wins       | ✅ Yes      | Single function, binary result, no scoring complexity                                                                                        |
| Add signs for Ralph   | ✅ Yes      | Light prompt additions, learning from code exploration                                                                                       |

---

### Ralph-Friendly Work Branches

_The Critical Principle:_ Geoff's Ralph works from a single, disposable plan where Ralph picks "most important." To use branches with Ralph while maintaining this pattern, you must scope at plan creation, not at task selection.

_Why this matters:_

- ❌ _Wrong approach_: Create full plan, then ask Ralph to "filter" tasks at runtime → unreliable (70-80%), violates determinism
- ✅ _Right approach_: Create a scoped plan upfront for each work branch → deterministic, simple, maintains "plan is disposable"

_Solution:_ Add a `plan-work` mode to create a work-scoped IMPLEMENTATION_PLAN.md on the current branch. User creates work branch, then runs `plan-work` with a natural language description of the work focus. The LLM uses this description to scope the plan. Post planning, Ralph builds from this already-scoped plan with zero semantic filtering - just picks "most important" as always.

_Terminology:_ "Work" is intentionally broad - it can describe features, topics of concern, refactoring efforts, infrastructure changes, bug fixes, or any coherent body of related changes. The work description you pass to `plan-work` is natural language for the LLM - it can be prose, not constrained by git branch naming rules.

#### Design Principles

- ✅ _Each Ralph session operates monolithically_ on ONE body of work per branch
- ✅ _User creates branches manually_ - full control over naming conventions and strategy (e.g. worktrees)
- ✅ _Natural language work descriptions_ - pass prose to LLM, unconstrained by git naming rules
- ✅ _Scoping at plan creation_ (deterministic) not task selection (probabilistic)
- ✅ _Single plan per branch_ - one IMPLEMENTATION_PLAN.md per branch
- ✅ _Plan remains disposable_ - regenerate scoped plan when wrong/stale for a branch
- ✅ No dynamic branch switching within a loop session
- ✅ Maintains simplicity and determinism
- ✅ Optional - main branch workflow still works
- ✅ No semantic filtering at build time - Ralph just picks "most important"

#### Workflow

_1. Full Planning (on main branch)_

```bash
./loop.sh plan
# Generate full IMPLEMENTATION_PLAN.md for entire project
```

_2. Create Work Branch_

User performs:

```bash
git checkout -b ralph/user-auth-oauth
# Create branch with whatever naming convention you prefer
# Suggestion: ralph/* prefix for work branches
```

_3. Scoped Planning (on work branch)_

```bash
./loop.sh plan-work "user authentication system with OAuth and session management"
# Pass natural language description - LLM uses this to scope the plan
# Creates focused IMPLEMENTATION_PLAN.md with only tasks for this work
```

_4. Build from Plan (on work branch)_

```bash
./loop.sh
# Ralph builds from scoped plan (no filtering needed)
# Picks most important task from already-scoped plan
```

_5. PR Creation (when work complete)_

User performs:

```bash
gh pr create --base main --head ralph/user-auth-oauth --fill
```

#### Work-Scoped Loop Script

Extends the base enhanced loop script to add work branch support with scoped planning:

```bash
#!/bin/bash
set -euo pipefail

# Usage:
#   ./loop.sh [plan] [max_iterations]       # Plan/build on current branch
#   ./loop.sh plan-work "work description"  # Create scoped plan on current branch
# Examples:
#   ./loop.sh                               # Build mode, unlimited
#   ./loop.sh 20                            # Build mode, max 20
#   ./loop.sh plan 5                        # Full planning, max 5
#   ./loop.sh plan-work "user auth"         # Scoped planning

# Parse arguments
MODE="build"
PROMPT_FILE="PROMPT_build.md"

if [ "$1" = "plan" ]; then
    # Full planning mode
    MODE="plan"
    PROMPT_FILE="PROMPT_plan.md"
    MAX_ITERATIONS=${2:-0}
elif [ "$1" = "plan-work" ]; then
    # Scoped planning mode
    if [ -z "$2" ]; then
        echo "Error: plan-work requires a work description"
        echo "Usage: ./loop.sh plan-work \"description of the work\""
        exit 1
    fi
    MODE="plan-work"
    WORK_DESCRIPTION="$2"
    PROMPT_FILE="PROMPT_plan_work.md"
    MAX_ITERATIONS=${3:-5}  # Default 5 for work planning
elif [[ "$1" =~ ^[0-9]+$ ]]; then
    # Build mode with max iterations
    MAX_ITERATIONS=$1
else
    # Build mode, unlimited
    MAX_ITERATIONS=0
fi

ITERATION=0
CURRENT_BRANCH=$(git branch --show-current)

# Validate branch for plan-work mode
if [ "$MODE" = "plan-work" ]; then
    if [ "$CURRENT_BRANCH" = "main" ] || [ "$CURRENT_BRANCH" = "master" ]; then
        echo "Error: plan-work should be run on a work branch, not main/master"
        echo "Create a work branch first: git checkout -b ralph/your-work"
        exit 1
    fi

    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Mode:    plan-work"
    echo "Branch:  $CURRENT_BRANCH"
    echo "Work:    $WORK_DESCRIPTION"
    echo "Prompt:  $PROMPT_FILE"
    echo "Plan:    Will create scoped IMPLEMENTATION_PLAN.md"
    [ "$MAX_ITERATIONS" -gt 0 ] && echo "Max:     $MAX_ITERATIONS iterations"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    # Warn about uncommitted changes to IMPLEMENTATION_PLAN.md
    if [ -f "IMPLEMENTATION_PLAN.md" ] && ! git diff --quiet IMPLEMENTATION_PLAN.md 2>/dev/null; then
        echo "Warning: IMPLEMENTATION_PLAN.md has uncommitted changes that will be overwritten"
        read -p "Continue? [y/N] " -n 1 -r
        echo
        [[ ! $REPLY =~ ^[Yy]$ ]] && exit 1
    fi

    # Export work description for PROMPT_plan_work.md
    export WORK_SCOPE="$WORK_DESCRIPTION"
else
    # Normal plan/build mode
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Mode:   $MODE"
    echo "Branch: $CURRENT_BRANCH"
    echo "Prompt: $PROMPT_FILE"
    echo "Plan:   IMPLEMENTATION_PLAN.md"
    [ "$MAX_ITERATIONS" -gt 0 ] && echo "Max:    $MAX_ITERATIONS iterations"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
fi

# Verify prompt file exists
if [ ! -f "$PROMPT_FILE" ]; then
    echo "Error: $PROMPT_FILE not found"
    exit 1
fi

# Main loop
while true; do
    if [ "$MAX_ITERATIONS" -gt 0 ] && [ "$ITERATION" -ge "$MAX_ITERATIONS" ]; then
        echo "Reached max iterations: $MAX_ITERATIONS"

        if [ "$MODE" = "plan-work" ]; then
            echo ""
            echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
            echo "Scoped plan created: $WORK_DESCRIPTION"
            echo "To build, run:"
            echo "  ./loop.sh 20"
            echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        fi
        break
    fi

    # Run Ralph iteration with selected prompt
    # -p: Headless mode (non-interactive, reads from stdin)
    # --dangerously-skip-permissions: Auto-approve all tool calls (YOLO mode)
    # --output-format=stream-json: Structured output for logging/monitoring
    # --model opus: Primary agent uses Opus for complex reasoning (task selection, prioritization)
    #               Can use 'sonnet' for speed if plan is clear and tasks well-defined
    # --verbose: Detailed execution logging

    # For plan-work mode, substitute ${WORK_SCOPE} in prompt before piping
    if [ "$MODE" = "plan-work" ]; then
        envsubst < "$PROMPT_FILE" | claude -p \
            --dangerously-skip-permissions \
            --output-format=stream-json \
            --model opus \
            --verbose
    else
        cat "$PROMPT_FILE" | claude -p \
            --dangerously-skip-permissions \
            --output-format=stream-json \
            --model opus \
            --verbose
    fi

    # Push to current branch
    CURRENT_BRANCH=$(git branch --show-current)
    git push origin "$CURRENT_BRANCH" || {
        echo "Failed to push. Creating remote branch..."
        git push -u origin "$CURRENT_BRANCH"
    }

    ITERATION=$((ITERATION + 1))
    echo -e "\n\n======================== LOOP $ITERATION ========================\n"
done
```

#### `PROMPT_plan_work.md` Template

_Note:_ Identical to `PROMPT_plan.md` but with scoping instructions and `WORK_SCOPE` env var substituted (automatically by the loop script).

```
0a. Study `specs/*` with up to 250 parallel Sonnet subagents to learn the application specifications.
0b. Study @IMPLEMENTATION_PLAN.md (if present) to understand the plan so far.
0c. Study `src/lib/*` with up to 250 parallel Sonnet subagents to understand shared utilities & components.
0d. For reference, the application source code is in `src/*`.

1. You are creating a SCOPED implementation plan for work: "${WORK_SCOPE}". Study @IMPLEMENTATION_PLAN.md (if present; it may be incorrect) and use up to 500 Sonnet subagents to study existing source code in `src/*` and compare it against `specs/*`. Use an Opus subagent to analyze findings, prioritize tasks, and create/update @IMPLEMENTATION_PLAN.md as a bullet point list sorted in priority of items yet to be implemented. Ultrathink. Consider searching for TODO, minimal implementations, placeholders, skipped/flaky tests, and inconsistent patterns. Study @IMPLEMENTATION_PLAN.md to determine starting point for research and keep it up to date with items considered complete/incomplete using subagents.

IMPORTANT: This is SCOPED PLANNING for "${WORK_SCOPE}" only. Create a plan containing ONLY tasks directly related to this work scope. Be conservative - if uncertain whether a task belongs to this work, exclude it. The plan can be regenerated if too narrow. Plan only. Do NOT implement anything. Do NOT assume functionality is missing; confirm with code search first. Treat `src/lib` as the project's standard library for shared utilities and components. Prefer consolidated, idiomatic implementations there over ad-hoc copies.

ULTIMATE GOAL: We want to achieve the scoped work "${WORK_SCOPE}". Consider missing elements related to this work and plan accordingly. If an element is missing, search first to confirm it doesn't exist, then if needed author the specification at specs/FILENAME.md. If you create a new element then document the plan to implement it in @IMPLEMENTATION_PLAN.md using a subagent.
```

#### Compatibility with Core Philosophy

| Principle              | Maintained? | How                                                                      |
| ---------------------- | ----------- | ------------------------------------------------------------------------ |
| Monolithic operation   | ✅ Yes      | Ralph still operates as single process within branch                     |
| One task per loop      | ✅ Yes      | Unchanged                                                                |
| Fresh context          | ✅ Yes      | Unchanged                                                                |
| Deterministic          | ✅ Yes      | Scoping at plan creation (deterministic), not runtime (prob.)            |
| Simple                 | ✅ Yes      | Optional enhancement, main workflow still works                          |
| Plan-driven            | ✅ Yes      | One IMPLEMENTATION_PLAN.md per branch                                    |
| Single source of truth | ✅ Yes      | One plan per branch - scoped plan replaces full plan on branch           |
| Plan is disposable     | ✅ Yes      | Regenerate scoped plan anytime: `./loop.sh plan-work "work description"` |
| Markdown over JSON     | ✅ Yes      | Still markdown plans                                                     |
| Let Ralph Ralph        | ✅ Yes      | Ralph picks "most important" from already-scoped plan - no filter        |

---

### JTBD → Story Map → SLC Release

#### Topics of Concern → Activities

Geoff's [suggested workflow](https://ghuntley.com/content/images/size/w2400/2025/07/The-ralph-Process.png) already aligns planning with Jobs-to-be-Done — breaking JTBDs into topics of concern, which in turn become specs. I love this and I think there's an opportunity to lean further into the product benefits this approach affords by reframing _topics of concern_ as _activities_.

Activities are verbs in a journey ("upload photo", "extract colors") rather than capabilities ("color extraction system"). They're naturally scoped by user intent.

> Topics: "color extraction", "layout engine" → capability-oriented
> Activities: "upload photo", "see extracted colors", "arrange layout" → journey-oriented

#### Activities → User Journey

Activities — and their constituent steps — sequence naturally into a user flow, creating a _journey structure_ that makes gaps and dependencies visible. A _[User Story Map](https://www.nngroup.com/articles/user-story-mapping/)_ organizes activities as columns (the journey backbone) with capability depths as rows — the full space of what _could_ be built:

```
UPLOAD    →   EXTRACT    →   ARRANGE     →   SHARE

basic         auto           manual          export
bulk          palette        templates       collab
batch         AI themes      auto-layout     embed
```

#### User Journey → Release Slices

Horizontal slices through the map become candidate releases. Not every activity needs new capability in every release — some cells stay empty, and that's fine if the slice is still coherent:

```
                  UPLOAD    →   EXTRACT    →   ARRANGE     →   SHARE

Release 1:        basic         auto                           export
                  ───────────────────────────────────────────────────
Release 2:                      palette        manual
                  ───────────────────────────────────────────────────
Release 3:        batch         AI themes      templates       embed
```

#### Release Slices → SLC Releases

The story map gives you _structure_ for slicing. Jason Cohen's _[Simple, Lovable, Complete (SLC)](https://longform.asmartbear.com/slc/)_ gives you _criteria_ for what makes a slice good:

- _Simple_ — Narrow scope you can ship fast. Not every activity, not every depth.
- _Complete_ — Fully accomplishes a job within that scope. Not a broken preview.
- _Lovable_ — People actually want to use it. Delightful within its boundaries.

_Why SLC over MVP?_ MVPs optimize for learning at the customer's expense — "minimum" often means broken or frustrating. SLC flips this: learn in-market _while_ delivering real value. If it succeeds, you have optionality. If it fails, you still treated users well.

Each slice can become a release with a clear value and identity:

```
                  UPLOAD    →   EXTRACT    →   ARRANGE     →   SHARE

Palette Picker:   basic         auto                           export
                  ───────────────────────────────────────────────────
Mood Board:                     palette        manual
                  ───────────────────────────────────────────────────
Design Studio:    batch         AI themes      templates       embed
```

- _Palette Picker_ — Upload, extract, export. Instant value from day one.
- _Mood Board_ — Adds arrangement. Creative expression enters the journey.
- _Design Studio_ — Professional features: batch processing, AI themes, embeddable output.

---

#### Operationalizing with Ralph

The concepts above — activities, story maps, SLC releases — are the _thinking tools_. How do we translate them into Ralph's workflow?

_Default Ralph approach:_

1. _Define Requirements_: Human + LLM define JTBD topics of concern → `specs/*.md`
2. _Create Tasks Plan_: LLM analyzes all specs + current code → `IMPLEMENTATION_PLAN.md`
3. _Build_: Ralph builds against full scope

This works well for capability-focused work (features, refactors, infrastructure). But it doesn't naturally produce valuable (SLC) product releases - it produces "whatever the specs describe".

_Activities → SLC Release approach:_

To get SLC releases, we need to ground activities in audience context. Audience defines WHO has the JTBDs, which in turn informs WHAT activities matter and what "lovable" means.

```
Audience (who)
    └── has JTBDs (desired outcomes)
            └── fulfilled by Activities (means to achieve outcomes)
```

##### Workflow

_I. Requirements Phase (2 steps):_

Still performed in LLM conversations with the human, similar to the default Ralph approach.

1. _Define audience and their JTBDs_ — WHO are we building for and what OUTCOMES do they want?

   - Human + LLM discuss and determine the audience(s) and their JTBDs (outcomes they want)
   - May contain multiple connected audiences (e.g. "designer" creates, "client" reviews)
   - Generates `AUDIENCE_JTBD.md`

2. _Define activities_ — WHAT do users do to accomplish their JTBDs?

   - Informed by `AUDIENCE_JTBD.md`
   - For each JTBD, identify activities necessary to accomplish it
   - For each activity, determine:
     - Capability depths (basic → enhanced) — levels of sophistication
     - Desired outcome(s) at each depth — what does success look like?
   - Generates `specs/*.md` (one per activity)

   The discrete steps within activities are implicit and LLM can infer them during planning.

_II. Planning Phase:_

Performed in Ralph loop with _updated_ planning prompt.

- LLM analyzes:
  - `AUDIENCE_JTBD.md` (who, desired outcomes)
  - `specs/*` (what could be built)
  - Current code state (what exists)
- LLM determines next SLC slice (which activities, at what capability depths) and plans tasks for that slice
- LLM generates `IMPLEMENTATION_PLAN.md`
- _Human verifies_ plan before building:
  - Does the scope represent a coherent SLC release?
  - Are the right activities included at the right depths?
  - If wrong → re-run planning loop to regenerate plan, optionally updating inputs or planning prompt
  - If right → proceed to building

_III. Building Phase:_

Performed in Ralph loop with standard building prompt.

##### Updated Planning Prompt

Variant of `PROMPT_plan.md` that adds audience context and SLC-oriented slice recommendation.

_Notes:_

- Unlike the default template, this does not have a `[project-specific goal]` placeholder — the goal is implicit: recommend the most valuable next release for the audience.
- Current subagents names presume using Claude.

```
0a. Study @AUDIENCE_JTBD.md to understand who we're building for and their Jobs to Be Done.
0b. Study `specs/*` with up to 250 parallel Sonnet subagents to learn JTBD activities.
0c. Study @IMPLEMENTATION_PLAN.md (if present) to understand the plan so far.
0d. Study `src/lib/*` with up to 250 parallel Sonnet subagents to understand shared utilities & components.
0e. For reference, the application source code is in `src/*`.

1. Sequence the activities in `specs/*` into a user journey map for the audience in @AUDIENCE_JTBD.md. Consider how activities flow into each other and what dependencies exist.

2. Determine the next SLC release. Use up to 500 Sonnet subagents to compare `src/*` against `specs/*`. Use an Opus subagent to analyze findings. Ultrathink. Given what's already implemented recommend which activities (at what capability depths) form the most valuable next release. Prefer thin horizontal slices - the narrowest scope that still delivers real value. A good slice is Simple (narrow, achievable), Lovable (people want to use it), and Complete (fully accomplishes a meaningful job, not a broken preview).

3. Use an Opus subagent (ultrathink) to analyze and synthesize the findings, prioritize tasks, and create/update @IMPLEMENTATION_PLAN.md as a bullet point list sorted in priority of items yet to be implemented for the recommended SLC release. Begin plan with a summary of the recommended SLC release (what's included and why), then list prioritized tasks for that scope. Consider TODOs, placeholders, minimal implementations, skipped tests - but scoped to the release. Note discoveries outside scope as future work.

IMPORTANT: Plan only. Do NOT implement anything. Do NOT assume functionality is missing; confirm with code search first. Treat `src/lib` as the project's standard library for shared utilities and components. Prefer consolidated, idiomatic implementations there over ad-hoc copies.

ULTIMATE GOAL: We want to achieve the most valuable next release for the audience in @AUDIENCE_JTBD.md. Consider missing elements and plan accordingly. If an element is missing, search first to confirm it doesn't exist, then if needed author the specification at specs/FILENAME.md. If you create a new element then document the plan to implement it in @IMPLEMENTATION_PLAN.md using a subagent.
```

##### Notes

_Why `AUDIENCE_JTBD.md` as a separate artifact:_

- Single source of truth — prevents drift across specs
- Enables holistic reasoning: "What does this audience need MOST?"
- JTBDs captured alongside audience (the "why" lives with the "who")
- Referenced twice: during spec creation AND SLC planning
- Keeps activity specs focused on WHAT, not repeating WHO

_Cardinalities:_

- One audience → many JTBDs ("Designer" has "capture space", "explore concepts", "present to client")
- One JTBD → many activities ("capture space" includes upload, measurements, room detection)
- One activity → can serve multiple JTBDs ("upload photo" serves both "capture" and "gather inspiration")
