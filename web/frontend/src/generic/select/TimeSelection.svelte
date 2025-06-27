<!--
    @component Selector for specified real time ranges for data cutoff; used in systems and nodes view

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

  /* Svelte 5 Props */
  let {
    presetFrom,
    presetTo,
    customEnabled = true,
    options = {
      "Last quarter hour": 15 * 60,
      "Last half hour": 30 * 60,
      "Last hour": 60 * 60,
      "Last 2hrs": 2 * 60 * 60,
      "Last 4hrs": 4 * 60 * 60,
      "Last 12hrs": 12 * 60 * 60,
      "Last 24hrs": 24 * 60 * 60,
    },
    applyTime
  } = $props();

  /* Const Init */
  const defaultTo = new Date(Date.now());
  const defaultFrom = new Date(defaultTo.setHours(defaultTo.getHours() - 4));

  /* State Init */
  let timeType = $state("range");
  let pendingCustomFrom = $state(null);
  let pendingCustomTo = $state(null);

  /* Derived */
  let timeRange = $derived.by(() => {
    if (presetTo && presetFrom) {
      return ((presetTo.getTime() - presetFrom.getTime()) / 1000)
    } else {
      return ((defaultTo.getTime() - defaultFrom.getTime()) / 1000)
    }
  });
  let unknownRange = $derived(!Object.values(options).includes(timeRange));
    
  /* Functions */
  function updateTimeRange() {
    let now = Date.now();
    let t = timeRange * 1000;
    applyTime(new Date(now - t), new Date(now));
  };

  function updateTimeCustom() {
    if (pendingCustomFrom && pendingCustomTo) {
      applyTime(new Date(pendingCustomFrom), new Date(pendingCustomTo));
    }
  };
</script>

<InputGroup class="inline-from">
  <InputGroupText><Icon name="clock-history" /></InputGroupText>
  {#if customEnabled}
    <Input
      type="select"
      style="max-width:fit-content;background-color:#f8f9fa;"
      bind:value={timeType}
    >
      <option value="range">Range</option>
      <option value="custom">Custom</option>
    </Input>
  {:else}
    <InputGroupText>Range</InputGroupText>
  {/if}
  
  {#if timeType === "range"}
    <Input
      type="select"
      bind:value={timeRange}
      onchange={updateTimeRange}
    >
      {#if unknownRange}
        <option value={timeRange} disabled>Select new range...</option>
      {/if}
      {#each Object.entries(options) as [name, seconds]}
        <option value={seconds}>{name}</option>
      {/each}
    </Input>
  {:else}
    <InputGroupText>from</InputGroupText>
    <Input
      type="datetime-local"
      bind:value={pendingCustomFrom}
      onchange={updateTimeCustom}
    ></Input>
    <InputGroupText>to</InputGroupText>
    <Input
      type="datetime-local"
      bind:value={pendingCustomTo}
      onchange={updateTimeCustom}
    ></Input>
  {/if}
</InputGroup>
