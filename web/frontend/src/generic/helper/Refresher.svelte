<!-- 
    @component Triggers upstream data refresh in selectable intervals

    Properties:
    - `initially Number?`: Initial refresh interval on component mount, in seconds [Default: null]

    Events:
    - `refresh`: When fired, the upstream component refreshes its contents
 -->
<script>
  import { Button, Icon, Input, InputGroup } from "@sveltestrap/sveltestrap";

  /* Svelte 5 Props */
  let {
    initially = null,
    presetClass = "",
    onRefresh
  } = $props();

  /* State Init */
  let refreshInterval = $state(initially ? initially * 1000 : null);

  /* Var Init */
  let refreshIntervalId = null;

  /* Functions */
  function refreshIntervalChanged() {
    if (refreshIntervalId != null) clearInterval(refreshIntervalId);
    if (refreshInterval == null) return;
    refreshIntervalId = setInterval(() => onRefresh(), refreshInterval);
  }

  /* Svelte 5 onMount */
  $effect(() => {
    refreshIntervalChanged();
  });
</script>

<InputGroup class={presetClass}>
  <Input
    type="select"
    title="Periodic refresh interval"
    bind:value={refreshInterval}
    onchange={refreshIntervalChanged}
  >
    <option value={null}>No Interval</option>
    <option value={30 * 1000}>30 Seconds</option>
    <option value={60 * 1000}>60 Seconds</option>
    <option value={2 * 60 * 1000}>Two Minutes</option>
    <option value={5 * 60 * 1000}>5 Minutes</option>
  </Input>
  <Button
    outline
    onclick={() => onRefresh()}
    disabled={refreshInterval != null}
    >
    <Icon name="arrow-clockwise" /> Refresh
  </Button>
</InputGroup>

