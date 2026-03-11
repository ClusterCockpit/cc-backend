# Session Context

## User Prompts

### Prompt 1

Implement the following plan:

# Make SQLite Memory Limits Configurable via config.json

## Context

Fixes 1-4 for the SQLite memory leak are already implemented on this branch. The hardcoded defaults (200MB cache per connection, 1GB soft heap limit) are conservative. On the production server with 512GB RAM, these could be tuned higher for better query performance. Additionally, `RepositoryConfig` and `SetConfig()` exist but are **never wired up** — there's currently no way to override any re...

