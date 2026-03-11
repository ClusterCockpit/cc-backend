# Session Context

## User Prompts

### Prompt 1

Implement the following plan:

# Fix SQLite Memory Not Released After Query Timeout

## Context

On the production 20M-row database, when a query runs into a timeout (due to full-table scan with wrong index), the memory allocated by SQLite is **not released afterwards**. The process stays bloated until restarted. This is caused by three compounding issues in the current SQLite configuration.

## Root Cause Analysis

### 1. `_cache_size=1000000000` is effectively unlimited (~4TB)

**File:** `i...

### Prompt 2

Our server has 512GB main memory. Does it make sense to make cache_size and soft_heap_limit configurable to make use of the main memory capacity?

