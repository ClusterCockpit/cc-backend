<!--
  @component Pie Plot based on chart.js Pie

  Properties:
  - `canvasId String?`: Unique ID for correct parallel chart.js rendering [Default: "pie-default"]
  - `size Number`: X and Y size of the plot, for square shape
  - `sliceLabel String`: Label used in segment legends
  - `quantities [Number]`: Data values
  - `entities [String]`: Data identifiers
  - `displayLegend?`: Display uPlot legend [Default: false]

  Exported:
  - `colors ['rgb(x,y,z)', ...]`: Color range used for segments; upstream used for legend
-->

<script module>
  export const colors = {
    // https://www.learnui.design/tools/data-color-picker.html#divergent: 11, Shallow Green-Red
    default: [
      "#00876c",
      "#449c6e",
      "#70af6f",
      "#9bc271",
      "#c8d377",
      "#f7e382",
      "#f6c468",
      "#f3a457",
      "#ed834e",
      "#e3614d",
      "#d43d51",
    ],
    // https://www.learnui.design/tools/data-color-picker.html#palette: 12, Colorwheel-Like
    alternative: [
      "#0022bb",
      "#ba0098",
      "#fa0066",
      "#ff6234",
      "#ffae00",
      "#b1af00",
      "#67a630",
      "#009753",
      "#00836c",
      "#006d77",
      "#005671",
      "#003f5c",
    ],
    // http://tsitsul.in/blog/coloropt/ : 12 colors normal
    colorblind: [
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
      'rgb(189,189,189)',
    ],
    nodeStates: {
      allocated: "rgba(0, 128, 0, 0.75)",
      down: "rgba(255, 0, 0, 0.75)",
      idle: "rgba(0, 0, 255, 0.75)",
      reserved: "rgba(255, 0, 255, 0.75)",
      mixed: "rgba(255, 215, 0, 0.75)",
      unknown: "rgba(0, 0, 0, 0.75)"
    }
  }
</script>

<script>
  // Ignore VSC IDE "One Instance Level Script" Error
  import { onMount, getContext } from "svelte";
  import Chart from 'chart.js/auto';

  /* Svelte 5 Props */
  let {
    canvasId = "pie-default",
    size,
    sliceLabel,
    quantities,
    entities,
    displayLegend = false,
    useAltColors = false,
    fixColors = null
  } = $props();

  /* Const Init */
  const useCbColors = getContext("cc-config")?.plotConfiguration_colorblindMode || false
  const options = { 
    maintainAspectRatio: false,
    animation: false,
    plugins: {
      legend: {
        // svelte-ignore state_referenced_locally
        display: displayLegend
      }
    }
  };

  /* Derived */
  const colorPalette = $derived.by(() => {
    let c;
    if (useCbColors) {
      c = [...colors['colorblind']];
    } else if (useAltColors) {
      c = [...colors['alternative']];
    } else if (fixColors?.length > 0) {
      c = [...fixColors];
    } else {
      c = [...colors['default']];
    }
    return c.slice(0, quantities.length);
  })

  const data = $derived({
    labels: entities,
    datasets: [
      {
        label: sliceLabel,
        data: quantities,
        fill: 1,
        backgroundColor: colorPalette,
      }
    ]
  });

  /* On Mount */
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
<div class="chart-container" style="--container-width: {size}px; --container-height: {size}px">
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
