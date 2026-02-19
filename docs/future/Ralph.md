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
