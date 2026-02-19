# Ollama + Claude Code Integration

**Research Date:** January 25, 2026
**Source:** [Ollama Blog - Claude](https://ollama.com/blog/claude)

## Overview

Ollama v0.14.0+ now supports the **Anthropic Messages API**, enabling Claude Code to work with local open-source models instead of requiring Anthropic's cloud API.

## Key Benefits

| Benefit | Description |
|---------|-------------|
| **Privacy** | Data stays on your local machine |
| **Cost Savings** | No API fees for extended coding sessions |
| **Offline Capability** | Works in air-gapped environments |
| **Flexibility** | Switch between local and cloud models seamlessly |

## Setup Instructions

### Prerequisites

- Ollama v0.14.2 or higher

### Configuration

Set the following environment variables:

```bash
export ANTHROPIC_BASE_URL=http://localhost:11434
export ANTHROPIC_API_KEY=ollama
```

### Running Claude Code with Local Models

**Basic usage:**
```bash
claude --model qwen3-coder
```

**Inline configuration:**
```bash
ANTHROPIC_BASE_URL=http://localhost:11434 ANTHROPIC_API_KEY=ollama claude --model qwen3-coder
```

## Recommended Models

### Local Models

| Model | Use Case | Notes |
|-------|----------|-------|
| `qwen3-coder` | Code generation | Optimized for coding tasks |
| `gpt-oss:20b` | General purpose | Good balance of capability |

### Cloud Models (via ollama.com)

| Model | Use Case |
|-------|----------|
| `glm-4.7:cloud` | Advanced reasoning |
| `minimax-m2.1:cloud` | Multi-purpose |

### Context Window Requirements

Models should have at least **32K tokens** context window for optimal Claude Code performance.

## Python SDK Integration

Existing applications using the Anthropic SDK can connect to Ollama:

```python
import anthropic

client = anthropic.Anthropic(
    base_url='http://localhost:11434',
    api_key='ollama',  # Required but ignored by Ollama
)

message = client.messages.create(
    model="qwen3-coder",
    max_tokens=1024,
    messages=[
        {"role": "user", "content": "Write a Python function to calculate fibonacci numbers"}
    ]
)

print(message.content)
```

## Use Cases

1. **Privacy-sensitive projects** - Keep proprietary code local
2. **Air-gapped development** - No internet required after model download
3. **Cost optimization** - Reduce API costs for long development sessions
4. **Model experimentation** - Test different models without switching providers
5. **Offline development** - Continue working without connectivity

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      Claude Code                         │
│                (Agentic Coding Tool)                     │
└─────────────────────────┬───────────────────────────────┘
                          │
                          │ Anthropic Messages API
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│                        Ollama                            │
│              (Local Model Runtime)                       │
│                                                          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │ qwen3-coder │  │ gpt-oss:20b │  │   others    │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
└─────────────────────────────────────────────────────────┘
```

## Sources

- [Claude Code with Anthropic API compatibility - Ollama Blog](https://ollama.com/blog/claude)
- [Anthropic compatibility - Ollama Docs](https://docs.ollama.com/api/anthropic-compatibility)
- [Claude Code Integration - Ollama Docs](https://docs.ollama.com/integrations/claude-code)
- [LLM Gateway Configuration - Claude Code Docs](https://docs.anthropic.com/en/docs/claude-code/llm-gateway)
