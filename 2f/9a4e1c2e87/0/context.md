# Session Context

## User Prompts

### Prompt 1

Implement the following plan:

# Plan: Simplify Checkpoint and Cleanup Configuration

## Context

The metricstore checkpoint interval is always "12h" in practice and has no reason to be configurable. The cleanup interval for the "delete" mode already falls back to `retention-in-memory` when not set — this should be the fixed behavior for all modes. WAL is the preferred and more robust checkpoint format and should be the default instead of JSON.

These changes reduce unnecessary configuration ...

### Prompt 2

Make the checkpoints option also option also optional

