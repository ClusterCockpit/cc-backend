<!-- 
    @component

    Events:
    - 'reload': When fired, the parent component shoud refresh its contents
 -->
<script>
  import { createEventDispatcher } from "svelte";
  import { Button, Icon, InputGroup } from "@sveltestrap/sveltestrap";

  const dispatch = createEventDispatcher();

  let refreshInterval = null;
  let refreshIntervalId = null;
  function refreshIntervalChanged() {
    if (refreshIntervalId != null) clearInterval(refreshIntervalId);

    if (refreshInterval == null) return;

    refreshIntervalId = setInterval(() => dispatch("reload"), refreshInterval);
  }

  export let initially = null;
  if (initially != null) {
    refreshInterval = initially * 1000;
    refreshIntervalChanged();
  }
</script>

<InputGroup>
  <Button
    outline
    on:click={() => dispatch("reload")}
    disabled={refreshInterval != null}
  >
    <Icon name="arrow-clockwise" /> Reload
  </Button>
  <select
    class="form-select"
    bind:value={refreshInterval}
    on:change={refreshIntervalChanged}
  >
    <option value={null}>No periodic reload</option>
    <option value={30 * 1000}>Update every 30 seconds</option>
    <option value={60 * 1000}>Update every minute</option>
    <option value={2 * 60 * 1000}>Update every two minutes</option>
    <option value={5 * 60 * 1000}>Update every 5 minutes</option>
  </select>
</InputGroup>

