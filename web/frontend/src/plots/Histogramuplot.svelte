<!--
    @component
    Properties:
    - Todo
 -->

<script>
    import uPlot from 'uplot'
    import { formatNumber } from '../units.js'
    import { onMount, onDestroy } from 'svelte'
    import { Card } from 'sveltestrap'

    export let data
    export let width = 500
    export let height = 300
    export let xlabel = ''
    export let xunit = 'X'
    export let ylabel = ''
    export let yunit = 'Y'

    const { bars } = uPlot.paths

    const drawStyles = {
        bars:      1,
        points:    2,
    };

    function paths(u, seriesIdx, idx0, idx1, extendGap, buildClip) {
        let s = u.series[seriesIdx];
        let style = s.drawStyle;

        let renderer = (
            style == drawStyles.bars ? (
                bars({size: [0.75, 100]})
            ) :
            () => null
        )

        return renderer(u, seriesIdx, idx0, idx1, extendGap, buildClip);
    }

    let plotWrapper = null
    let legendWrapper = null
    let uplot = null
    let timeoutId = null
    
    function render() {
        let opts = {
            width: width,
            height: height,
            cursor: {
                points: {
                    size:   (u, seriesIdx)       => u.series[seriesIdx].points.size * 2.5,
                    width:  (u, seriesIdx, size) => size / 4,
                    stroke: (u, seriesIdx)       => u.series[seriesIdx].points.stroke(u, seriesIdx) + '90',
                    fill:   (u, seriesIdx)       => "#fff",
                }
            },
            scales: {
                x: {
                    time: false
                },
            },
            axes: [
                {
                    stroke: "#000000",
                    // scale: 'x',
                    label: xlabel,
                    labelGap: 10,
                    size: 25,
                    incrs: [1, 2, 5, 6, 10, 12, 50, 100, 500, 1000, 5000, 10000],
                    border: { 
                        show: true,
                        stroke: "#000000",
                    },
                    ticks: {
                        width: 1 / devicePixelRatio,
                        size: 5 / devicePixelRatio,
                        stroke: "#000000",
                    },
                    values: (_, t) => t.map(v => formatNumber(v)),
                },
                {
                    stroke: "#000000",
                    // scale: 'y',
                    label: ylabel,
                    labelGap: 10,
                    size: 35,
                    border: { 
                        show: true,
                        stroke: "#000000",
                    },
                    ticks: {
                        width: 1 / devicePixelRatio,
                        size: 5 / devicePixelRatio,
                        stroke: "#000000",
                    },
                    values: (_, t) => t.map(v => formatNumber(v)),
                },
            ],
            legend : {
                mount: (self, legend) => {
                    legendWrapper.appendChild(legend)
                },
                markers: {
                    show: false,
                    stroke: "#000000"
                }
            },
            series: [
                {
                    label: xunit,
                },
                Object.assign({
                    label: yunit,
                    width: 1 / devicePixelRatio,
                    drawStyle: drawStyles.points,
                    lineInterpolation: null,
                    paths,
                }, {
                    drawStyle: drawStyles.bars,
                    lineInterpolation: null,
                    stroke: "#85abce",
                    fill: "#85abce", //  + "1A", // Transparent Fill
                }),
            ]
        };

		uplot = new uPlot(opts, data, plotWrapper)
	}

    onMount(() => {
        render()
    })

    onDestroy(() => {
        if (uplot)
            uplot.destroy()

        if (timeoutId != null)
            clearTimeout(timeoutId)
    })

    function sizeChanged() {
        if (timeoutId != null)
            clearTimeout(timeoutId)

        timeoutId = setTimeout(() => {
            timeoutId = null
            if (uplot)
                uplot.destroy()

            render()
        }, 200)
    }

    $: sizeChanged(width, height)
</script>

{#if data.length > 0}
    <div bind:this={plotWrapper}>
        <div bind:this={legendWrapper}/>
    </div>
{:else}
    <Card class="mx-4" body color="warning">Cannot render histogram: No data!</Card>
{/if}


