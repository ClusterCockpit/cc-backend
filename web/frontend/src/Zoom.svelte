<script>
    import { Icon, InputGroup, InputGroupText } from 'sveltestrap';

    export let timeseriesPlots;

    let windowSize = 100; // Goes from 0 to 100
    let windowPosition = 50; // Goes from 0 to 100

    function updatePlots() {
        let ws = windowSize / (100 * 2),
            wp = windowPosition / 100;
        let from = (wp - ws),
            to = (wp + ws);
        Object
            .values(timeseriesPlots)
            .forEach(plot => plot.setTimeRange(from, to));
    }

    // Rendering a big job can take a long time, so we
    // throttle the rerenders to every 100ms here.
    let timeoutId = null;
    function requestUpdatePlots() {
        if (timeoutId != null)
            window.cancelAnimationFrame(timeoutId);

        timeoutId = window.requestAnimationFrame(() => {
            updatePlots();
            timeoutId = null;
        }, 100);
    }

    $: requestUpdatePlots(windowSize, windowPosition);
</script>

<div>
    <InputGroup>
        <InputGroupText>
            <Icon name="zoom-in"/>
        </InputGroupText>
        <InputGroupText>
            Window Size:
            <input
                style="margin: 0em 0em 0em 1em"
                type="range"
                bind:value={windowSize}
                min=1 max=100 step=1 />
            <span style="width: 5em;">
                ({windowSize}%)
            </span>
        </InputGroupText>
        <InputGroupText>
            Window Position:
            <input
                style="margin: 0em 0em 0em 1em"
                type="range"
                bind:value={windowPosition}
                min=0 max=100 step=1 />
        </InputGroupText>
    </InputGroup>
</div>
