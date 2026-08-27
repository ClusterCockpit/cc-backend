/*
 * Display-only smoothing for metric plots.
 *
 * This is deliberately separate from the backend resampling in cc-lib: those
 * algorithms decimate (they reduce the point count and their output length is
 * always shorter than their input), which makes them unusable as a filter.
 * A moving average has to keep every point in place, so it lives here and runs
 * after whatever downsampling the backend applied.
 */

/**
 * Centered, NaN-aware moving average. Preserves length and index alignment, so
 * the caller's X array and all derived index math stay valid.
 *
 * Windows are given in data points, not seconds. Values that are null or NaN
 * are skipped; a window containing nothing else yields NaN so uPlot keeps
 * rendering the gap. At the series edges the window shrinks instead of
 * producing NaN, which avoids introducing new gaps at the start and end.
 *
 * @param {Array<number|null>} data Input samples
 * @param {number} window Window width in data points; <= 1 disables smoothing
 * @returns {Array<number|null>} Smoothed copy, or `data` itself if disabled
 */
export function movingAverage(data, window) {
  if (!data || data.length === 0) return data;

  let w = Math.floor(Number(window));
  if (!Number.isFinite(w) || w <= 1) return data;
  if (w > data.length) w = data.length;
  // Force odd width so the window stays centered on the sample.
  if (w % 2 === 0) w -= 1;
  if (w <= 1) return data;

  const n = data.length;
  const h = (w - 1) / 2;
  const out = new Array(n);

  for (let i = 0; i < n; i++) {
    const start = i - h < 0 ? 0 : i - h;
    const end = i + h > n - 1 ? n - 1 : i + h;
    let sum = 0;
    let count = 0;
    for (let j = start; j <= end; j++) {
      const v = data[j];
      if (v == null || Number.isNaN(v)) continue;
      sum += v;
      count++;
    }
    out[i] = count === 0 ? NaN : sum / count;
  }

  return out;
}
