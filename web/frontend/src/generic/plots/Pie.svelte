<!--
    @component Pie Plot based on chart.js Pie

    Properties:
    - `size Number`: X and Y size of the plot, for square shape
    - `sliceLabel String`: Label used in segment legends
    - `quantities [Number]`: Data values
    - `entities [String]`: Data identifiers
    - `displayLegend?`: Display uPlot legend [Default: false]

    Exported:
    - `colors ['rgb(x,y,z)', ...]`: Color range used for segments; upstream used for legend
 -->

<script context="module">
    // http://tsitsul.in/blog/coloropt/ : 12 colors normal
    export const colors = [
        'rgb(235,172,35)',
        'rgb(184,0,88)',
        'rgb(0,140,249)',
        'rgb(0,110,0)',
        'rgb(0,187,173)',
        'rgb(209,99,230)',
        'rgb(178,69,2)',
        'rgb(255,146,135)',
        'rgb(89,84,214)',
        'rgb(0,198,248)',
        'rgb(135,133,0)',
        'rgb(0,167,108)',
        'rgb(189,189,189)'
    ]
</script>
<script>
    import Chart from 'chart.js/auto'
    import { onMount } from 'svelte';

    export let canvasId
    export let size
    export let sliceLabel
    export let quantities
    export let entities
    export let displayLegend = false

    const data = {
        labels: entities,
        datasets: [
            {
                label: sliceLabel,
                data: quantities,
                fill: 1,
                backgroundColor: colors.slice(0, quantities.length)
            }
        ]
    }

    const options = { 
        maintainAspectRatio: false,
        animation: false,
        plugins: {
            legend: {
                display: displayLegend
            }
        }
    }

    onMount(() => {
        new Chart(
            document.getElementById(canvasId),
            {
                type: 'pie',
                data: data,
                options: options
            }
        );
	});

</script>

<!-- <div style="width: 500px;"><canvas id="dimensions"></canvas></div><br/> -->
<div class="chart-container" style="--container-width: {size}; --container-height: {size}">
    <canvas id={canvasId}></canvas>
</div>

<style>
    .chart-container {
        position: relative;
        margin: auto;
        height: var(--container-height);
        width: var(--container-width);
    }
</style>
