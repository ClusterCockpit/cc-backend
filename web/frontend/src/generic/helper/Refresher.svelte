<!-- 
    @component Triggers upstream data refresh in selectable intervals

    Properties:
    - `initially Number?`: Initial refresh interval on component mount, in seconds [Default: null]

    Events:
    - `refresh`: When fired, the upstream component refreshes its contents
 -->
<script>
  import { Button, Icon, Input, InputGroup } from "@sveltestrap/sveltestrap";

  let refreshInterval = $state(null);
  let refreshIntervalId = null;

  function refreshIntervalChanged() {
    if (refreshIntervalId != null) clearInterval(refreshIntervalId);
    if (refreshInterval == null) return;
    refreshIntervalId = setInterval(() => dispatch("refresh"), refreshInterval);
  }

  let {
    initially = null,
    onRefresh
  } = $props();

  if (initially != null) {
    refreshInterval = initially * 1000;
    refreshIntervalChanged();
  }
</script>

<InputGroup>
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

