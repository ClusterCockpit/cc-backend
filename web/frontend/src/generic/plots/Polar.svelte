<!--
  @component Polar Plot based on chart.js Radar

  Properties:
  - `polarMetrics [Object]?`: Metric names and scaled peak values for rendering polar plot [Default: [] ]
  - `polarData [GraphQL.JobMetricStatWithName]?`: Metric data [Default: null]
  - `canvasId String?`: Unique ID for correct parallel chart.js rendering [Default: "polar-default"]
  - `showLegend Bool?`: Legend Display [Default: true]
-->

<script>
  import { onMount } from 'svelte'
  import Chart from 'chart.js/auto'
  import {
    Chart as ChartJS,
    Title,
    Tooltip,
    Legend,
    Filler,
    PointElement,
    RadialLinearScale,
    LineElement
  } from 'chart.js';

  /* Register Chart.js Components */
  ChartJS.register(
    Title,
    Tooltip,
    Legend,
    Filler,
    PointElement,
    RadialLinearScale,
    LineElement
  );

  /* Svelte 5 Props */
  let {
    polarMetrics = [],
    polarData = [],
    canvasId = "polar-default",
    size = 375,
    showLegend = true,
  } = $props();

  /* Derived */
  const options = $derived({
    responsive: true, // Default
    maintainAspectRatio: true, // Default
    animation: false,
    scales: { // fix scale
      r: {
        suggestedMin: 0.0,
        suggestedMax: 1.0
      }
    },
    plugins: {
      legend: {
        display: showLegend
      }
    }
  })

  const labels = $derived(polarMetrics
    .filter((m) => (m.peak != null))
    .map(pm => pm.name)
    .sort(function (a, b) {return ((a > b) ? 1 : ((b > a) ? -1 : 0))})
  );

  const data = $derived({
    labels: labels,
    datasets: [
      {
        label: 'Max',
        data: loadData('max'), // Node Scope Only
        fill: 1,
        backgroundColor: 'rgba(0, 0, 255, 0.25)',
        borderColor: 'rgb(0, 0, 255)',
        pointBackgroundColor: 'rgb(0, 0, 255)',
        pointBorderColor: '#fff',
        pointHoverBackgroundColor: '#fff',
        pointHoverBorderColor: 'rgb(0, 0, 255)'
      },
      {
        label: 'Avg',
        data: loadData('avg'), // Node Scope Only
        fill: true, // fill: 2 if min active
        backgroundColor: 'rgba(255, 210, 0, 0.25)',
        borderColor: 'rgb(255, 210, 0)',
        pointBackgroundColor: 'rgb(255, 210, 0)',
        pointBorderColor: '#fff',
        pointHoverBackgroundColor: '#fff',
        pointHoverBorderColor: 'rgb(255, 210, 0)'
      },
      // {
      //   label: 'Min',
      //   data: loadData('min'), // Node Scope Only
      //   fill: true,
      //   backgroundColor: 'rgba(255, 0, 0, 0.25)',
      //   borderColor: 'rgb(255, 0, 0)',
      //   pointBackgroundColor: 'rgb(255, 0, 0)',
      //   pointBorderColor: '#fff',
      //   pointHoverBackgroundColor: '#fff',
      //   pointHoverBorderColor: 'rgb(255, 0, 0)'
      // }
    ]
  });

  /* Functions */
  function loadData(type) {
    if (labels && (type == 'avg' || type == 'min' ||type == 'max')) {
      return getValues(type)
    } else if (!labels) {
      console.warn("Empty 'polarMetrics' array prop! Cannot render Polar representation.")
    } else {
      console.warn('Unknown Type For Polar Data (must be one of [min, max, avg])')
    }
    return []
  }

  // Helper
  const getValues = (type) => labels.map(name => {
    // Peak is adapted and scaled for job shared state
    const peak = polarMetrics.find(m => m?.name == name)?.peak
    const metric = polarData.find(m => m?.name == name)?.data
    const value = (peak && metric) ? (metric[type] / peak) : 0
    return value <= 1. ? value : 1.
  })

  /* On Mount */
  onMount(() => {
    new Chart(
      document.getElementById(canvasId),
      {
        type: 'radar',
        data: data,
        options: options
      }
    );
  });
</script>

<!-- <div style="width: 500px;"><canvas id="dimensions"></canvas></div><br/> -->
<div class="chart-container d-flex justify-content-center" style="--container-width: {size}px; --container-height: {size}px">
  <canvas id={canvasId}></canvas>
</div>

<style>
  .chart-container {
    margin: auto;
    position: relative;
    height: var(--container-height);
    width: var(--container-width);
  }
</style>