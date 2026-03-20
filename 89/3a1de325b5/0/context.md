# Session Context

## User Prompts

### Prompt 1

Implement the following plan:

# Plan: Add RRDTool-style Average Consolidation Function to Resampler

## Context

The current downsampler in `cc-lib/v2/resampler` offers two algorithms:
- **LTTB** (LargestTriangleThreeBucket): Perceptually-aware — picks points that preserve visual shape (peaks/valleys). Used at all call sites.
- **SimpleResampler**: Decimation — picks every nth point. Fast but lossy.

Neither produces scientifically accurate averages over time intervals. RRDTool's **AVERAGE C...

