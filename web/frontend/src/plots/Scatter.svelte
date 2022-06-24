<div class="cc-plot">
    <canvas bind:this={canvasElement} width="{width}" height="{height}"></canvas>
</div>

<script context="module">
    import { formatNumber } from '../utils.js'

    const axesColor = '#aaaaaa'
    const fontSize = 12
    const fontFamily = 'system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, "Noto Sans", sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol", "Noto Color Emoji"'
    const paddingLeft = 40,
        paddingRight = 10,
        paddingTop = 10,
        paddingBottom = 50

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

    function render(ctx, X, Y, S, color, xLabel, yLabel, width, height) {
        if (width <= 0)
            return;

        const [minX, minY] = [0., 0.];
        let maxX = X.reduce((maxX, x) => Math.max(maxX, x), minX);
        let maxY = Y.reduce((maxY, y) => Math.max(maxY, y), minY);
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
        if (S) {
            let max = S.reduce((max, s, i) => (X[i] == null || Y[i] == null || Number.isNaN(X[i]) || Number.isNaN(Y[i])) ? max : Math.max(max, s), 0)
            size = (w / 15) / max
        }

        ctx.fillStyle = color;
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
</script>

<script>
    import { onMount } from 'svelte';

    export let X;
    export let Y;
    export let S = null;
    export let color = '#0066cc';
    export let width;
    export let height;
    export let xLabel;
    export let yLabel;

    let ctx;
    let canvasElement;

    onMount(() => {
        canvasElement.width = width;
        canvasElement.height = height;
        ctx = canvasElement.getContext('2d');
        render(ctx, X, Y, S, color, xLabel, yLabel, width, height);
    });

    let timeoutId = null;
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

    $: sizeChanged(width, height);

</script>
