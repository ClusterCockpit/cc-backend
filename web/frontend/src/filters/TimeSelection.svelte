<!--
    @component Filter sub-component for selecting specified real time ranges for data cutoff; used in systems and nodes view

    Properties:
    - `from Date`: The datetime to start data display from
    - `to Date`: The datetime to end data display at
    - `customEnabled Bool?`: Allow custom time window selection [Default: true]
    - `options Object? {String:Number}`: The quick time selection options [Default: {..., "Last 24hrs": 24*60*60}]

    Events:
    - `change, {Date, Date}`: Set 'from, to' values in upstream component
 -->

<script>
  import {
    Icon,
    Input,
    InputGroup,
    InputGroupText,
  } from "@sveltestrap/sveltestrap";
  import { createEventDispatcher } from "svelte";

  export let from;
  export let to;
  export let customEnabled = true;
  export let options = {
    "Last quarter hour": 15 * 60,
    "Last half hour": 30 * 60,
    "Last hour": 60 * 60,
    "Last 2hrs": 2 * 60 * 60,
    "Last 4hrs": 4 * 60 * 60,
    "Last 12hrs": 12 * 60 * 60,
    "Last 24hrs": 24 * 60 * 60,
  };

  $: pendingFrom = from;
  $: pendingTo = to;

  const dispatch = createEventDispatcher();
  let timeRange = // If both times set, return diff, else: display custom select
    (to && from) ? ((to.getTime() - from.getTime()) / 1000) : -1;

  function updateTimeRange() {
    if (timeRange == -1) {
      pendingFrom = null;
      pendingTo = null;
      return;
    }

    let now = Date.now(),
      t = timeRange * 1000;
    from = pendingFrom = new Date(now - t);
    to = pendingTo = new Date(now);
    dispatch("change", { from, to });
  }

  function updateExplicitTimeRange(type, event) {
    let d = new Date(Date.parse(event.target.value));
    if (type == "from") pendingFrom = d;
    else pendingTo = d;

    if (pendingFrom != null && pendingTo != null) {
      from = pendingFrom;
      to = pendingTo;
      dispatch("change", { from, to });
    }
  }
</script>

<InputGroup class="inline-from">
  <InputGroupText><Icon name="clock-history" /></InputGroupText>
  <select
    class="form-select"
    bind:value={timeRange}
    on:change={updateTimeRange}
  >
    {#if customEnabled}
      <option value={-1}>Custom</option>
    {/if}
    {#each Object.entries(options) as [name, seconds]}
      <option value={seconds}>{name}</option>
    {/each}
  </select>
  {#if timeRange == -1}
    <InputGroupText>from</InputGroupText>
    <Input
      type="datetime-local"
      on:change={(event) => updateExplicitTimeRange("from", event)}
    ></Input>
    <InputGroupText>to</InputGroupText>
    <Input
      type="datetime-local"
      on:change={(event) => updateExplicitTimeRange("to", event)}
    ></Input>
  {/if}
</InputGroup>
