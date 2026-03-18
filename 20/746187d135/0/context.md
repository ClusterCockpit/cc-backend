# Session Context

## User Prompts

### Prompt 1

Implement the following plan:

# Plan: Improve GetUser logging

## Context

`GetUser` in `internal/repository/user.go` (line 75) logs a `Warn` for every query error, including the common `sql.ErrNoRows` case (user not found). Two problems:

1. `sql.ErrNoRows` is a normal, expected condition — many callers check for it explicitly. It should not produce a warning.
2. The log message **omits the actual error**: `"Error while querying user '%v' from database"` gives no clue what went wrong for re...

