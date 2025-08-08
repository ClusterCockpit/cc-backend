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
  // https://www.learnui.design/tools/data-color-picker.html#divergent: 11, Shallow Green-Red
  export const colors = [
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
  ];
  // https://www.learnui.design/tools/data-color-picker.html#palette: 12, Colorwheel-Like
  export const altColors = [
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
  ]
  // http://tsitsul.in/blog/coloropt/ : 12 colors normal
  export const cbColors = [
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
  ];
</script>

<script>
  /* Ignore Double Script Section Error in IDE */
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
  } = $props();

  /* Const Init */
  const ccconfig = getContext("cc-config");
  const options = { 
    maintainAspectRatio: false,
    animation: false,
    plugins: {
      legend: {
        display: displayLegend
      }
    }
  };

  /* State Init */
  let cbmode = $state(ccconfig?.plot_general_colorblindMode || false)

  /* Derived */
  const data = $derived({
    labels: entities,
    datasets: [
      {
        label: sliceLabel,
        data: quantities,
        fill: 1,
        backgroundColor: cbmode ? cbColors.slice(0, quantities.length) : colors.slice(0, quantities.length)
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
