<!--
  @component Scatter plot of two metrics at identical timesteps, based on canvas

  Properties:
  - `X [Number]`: Data from first selected metric as X-values
  - `Y [Number]`: Data from second selected metric as Y-values
  - `S GraphQl.TimeWeights.X?`: Float to scale the data with [Default: null]
  - `color String?`: Color of the drawn scatter circles [Default: '#0066cc']
  - `width Number?`: Width of the plot [Default: 250]
  - `height Number?`: Height of the plot [Default: 300]
  - `xLabel String?`: X-Axis Label [Ðefault: ""]
  - `yLabel String?`: Y-Axis Label [Default: ""]
-->

<script>
  import { onMount } from 'svelte';
  import { formatNumber } from '../units.js'

  /* Svelte 5 Props */
  let {
    X,
    Y,
    S = null,
    color = '#0066cc',
    width = 250,
    height = 300,
    xLabel = "",
    yLabel = "",
  } = $props();

  /* Const Init */
  const axesColor = '#aaaaaa';
  const fontSize = 12;
  const fontFamily = 'system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, "Noto Sans", sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol", "Noto Color Emoji"';
  const paddingLeft = 40;
  const paddingRight = 10;
  const paddingTop = 10;
  const paddingBottom = 50;

  /* Var Init */
  let timeoutId = null;

  /* State Init */
  let ctx = $state();
  let canvasElement = $state();

  /* Effects */
  $effect(() => {
    sizeChanged(width, height);
  });

  /* Functions */
  function getStepSize(valueRange, pixelRange, minSpace) {
    const proposition = valueRange / (pixelRange / minSpace);
    const getStepSize = n => Math.pow(10, Math.floor(n / 3)) *
      (n < 0 ? [1., 5., 2.][-n % 3] : [1., 2., 5.][n % 3]);

    let n = 0;
    let stepsize = getStepSize(n);
    while (true) {
      let bigger = getStepSize(n + 1);
      if (proposition > bigger) {
        n += 1;
        stepsize = bigger;
      } else {
        return stepsize;
      }
    }
  }

  function sizeChanged() {
    if (timeoutId != null)
      clearTimeout(timeoutId);

    timeoutId = setTimeout(() => {
      timeoutId = null;
      if (!canvasElement)
        return;

      canvasElement.width = width;
      canvasElement.height = height;
      ctx = canvasElement.getContext('2d');
      render(ctx, X, Y, S, color, xLabel, yLabel, width, height);
    }, 250);
  }

  function render(ctx, X, Y, S, color, xLabel, yLabel, width, height) {
    if (width <= 0)
      return;

    const [minX, minY] = [0., 0.];
    let maxX = X ? X.reduce((maxX, x) => Math.max(maxX, x), minX) : 1.0;
    let maxY = Y ? Y.reduce((maxY, y) => Math.max(maxY, y), minY) : 1.0;
    const w = width - paddingLeft - paddingRight;
    const h = height - paddingTop - paddingBottom;

    if (maxX == 0 && maxY == 0) {
      maxX = 1;
      maxY = 1;
    }

    /* Value -> Pixel-Coordinate */
    const getCanvasX = (x) => {
      x -= minX; x /= (maxX - minX);
      return Math.round((x * w) + paddingLeft);
    };
    const getCanvasY = (y) => {
      y -= minY; y /= (maxY - minY);
      return Math.round((h - y * h) + paddingTop);
    };

    // Draw Data
    let size = 3
    if (S && X && Y) {
      let max = S.reduce((max, s, i) => (X[i] == null || Y[i] == null || Number.isNaN(X[i]) || Number.isNaN(Y[i])) ? max : Math.max(max, s), 0)
      size = (w / 15) / max
    }

    ctx.fillStyle = color;
    if (X?.length > 0) {
      for (let i = 0; i < X.length; i++) {
        let x = X[i], y = Y[i];
        if (x == null || y == null || Number.isNaN(x) || Number.isNaN(y))
          continue;

        const s = S ? S[i] * size : size;
        const px = getCanvasX(x);
        const py = getCanvasY(y);

        ctx.beginPath();
        ctx.arc(px, py, s, 0, Math.PI * 2, false);
        ctx.fill();
      }
    }

    // Axes
    ctx.fillStyle = '#000000'
    ctx.strokeStyle = axesColor;
    ctx.font = `${fontSize}px ${fontFamily}`;
    ctx.beginPath();
    const stepsizeX = getStepSize(maxX, w, 75);
    for (let x = minX, i = 0; x <= maxX; i++) {
      let px = getCanvasX(x);
      let text = formatNumber(x);
      let textWidth = ctx.measureText(text).width;
      ctx.fillText(text,
        Math.floor(px - (textWidth / 2)),
        height - paddingBottom + fontSize + 5);
      ctx.moveTo(px, paddingTop - 5);
      ctx.lineTo(px, height - paddingBottom + 5);

      x += stepsizeX;
    }
    if (xLabel) {
      let textWidth = ctx.measureText(xLabel).width;
      ctx.fillText(xLabel, Math.floor((width / 2) - (textWidth / 2)), height - 20);
    }

    ctx.textAlign = 'center';
    const stepsizeY = getStepSize(maxY, h, 75);
    for (let y = minY, i = 0; y <= maxY; i++) {
      let py = getCanvasY(y);
      ctx.moveTo(paddingLeft - 5, py);
      ctx.lineTo(width - paddingRight + 5, py);

      ctx.save();
      ctx.translate(paddingLeft - 10, py);
      ctx.rotate(-Math.PI / 2);
      ctx.fillText(formatNumber(y), 0, 0);
      ctx.restore();

      y += stepsizeY;
    }
    if (yLabel) {
      ctx.save();
      ctx.translate(15, Math.floor(height / 2));
      ctx.rotate(-Math.PI / 2);
      ctx.fillText(yLabel, 0, 0);
      ctx.restore();
    }
    ctx.stroke();
  }

  /* On Mount */
  onMount(() => {
    canvasElement.width = width;
    canvasElement.height = height;
    ctx = canvasElement.getContext('2d');
    render(ctx, X, Y, S, color, xLabel, yLabel, width, height);
  });
</script>

<div class="cc-plot" bind:clientWidth={width}>
  <canvas bind:this={canvasElement}  {width} {height}></canvas>
</div>
