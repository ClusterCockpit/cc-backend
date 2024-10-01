<!--
    @component Filter sub-component for selecting job statistics

    Properties:
    - `isModified Bool?`: Is this filter component modified [Default: false]
    - `isOpen Bool?`: Is this filter component opened [Default: false]
    - `stats [Object]?`: The currently selected statistics filter [Default: []]

    Events:
    - `set-filter, {[Object]}`: Set 'stats' filter in upstream component
 -->

<script>
  import { createEventDispatcher, getContext } from "svelte";
  import { getStatsItems } from "../utils.js";
  import {
    Button,
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
  } from "@sveltestrap/sveltestrap";
  import DoubleRangeSlider from "../select/DoubleRangeSlider.svelte";

  const initialized = getContext("initialized"),
    dispatch = createEventDispatcher();

  export let isModified = false;
  export let isOpen = false;
  export let stats = [];

  let statistics = []
  function loadRanges(isInitialized) {
    if (!isInitialized) return;
    statistics = getStatsItems();
  }

  function resetRanges() {
    for (let st of statistics) {
      st.enabled = false
      st.from = 0
      st.to = st.peak
    } 
  }

  $: isModified = !statistics.every((a) => {
    let b = stats.find((s) => s.field == a.field);
    if (b == null) return !a.enabled;

    return a.from == b.from && a.to == b.to;
  });

  $: loadRanges($initialized);

</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Filter based on statistics</ModalHeader>
  <ModalBody>
    {#each statistics as stat}
      <h4>{stat.text}</h4>
      <DoubleRangeSlider
        on:change={({ detail }) => (
          (stat.from = detail[0]), (stat.to = detail[1]), (stat.enabled = true)
        )}
        min={0}
        max={stat.peak}
        firstSlider={stat.from}
        secondSlider={stat.to}
        inputFieldFrom={stat.from}
        inputFieldTo={stat.to}
      />
    {/each}
  </ModalBody>
  <ModalFooter>
    <Button
      color="primary"
      on:click={() => {
        isOpen = false;
        stats = statistics.filter((stat) => stat.enabled);
        dispatch("set-filter", { stats });
      }}>Close & Apply</Button
    >
    <Button
      color="danger"
      on:click={() => {
        isOpen = false;
        resetRanges();
        stats = [];
        dispatch("set-filter", { stats });
      }}>Reset</Button
    >
    <Button on:click={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
