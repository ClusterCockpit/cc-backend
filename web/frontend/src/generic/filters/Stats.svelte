<!--
    @component Filter sub-component for selecting job statistics

    Properties:
    - `isOpen Bool?`: Is this filter component opened [Default: false]
    - `stats [Object]?`: The currently selected statistics filter [Default: []]

    Events:
    - `set-filter, {[Object]}`: Set 'stats' filter in upstream component
 -->

<script>
  import { getStatsItems } from "../utils.js";
  import {
    Button,
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
  } from "@sveltestrap/sveltestrap";
  import DoubleRangeSlider from "../select/DoubleRangeSlider.svelte";

  /* Svelte 5 Props */
  let { 
    isOpen = $bindable(),
    presetStats,
    setFilter
   } = $props();

  /* Derived Init */
  const availableStats = $derived(getStatsItems(presetStats));

  /* Functions */
  function resetRanges() {
    for (let as of availableStats) {
      as.enabled = false
      as.from = 0
      as.to = as.peak
    };
  }
</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>
    <span>Filter based on statistics</span>
  </ModalHeader>
  <ModalBody>
    {#each availableStats as aStat}
      <div class="mb-3">
        <div class="mb-0"><b>{aStat.text}</b></div>
        <DoubleRangeSlider
          changeRange={(detail) => {
            aStat.from = detail[0];
            aStat.to = detail[1];
            if (aStat.from == 0 && aStat.to == aStat.peak) {
              aStat.enabled = false;
            } else {
              aStat.enabled = true;
            }
          }}
          sliderMin={0.0}
          sliderMax={aStat.peak}
          fromPreset={aStat.from}
          toPreset={aStat.to}
        />
      </div>
    {/each}
  </ModalBody>
  <ModalFooter>
    <Button
      color="primary"
      onclick={() => {
        isOpen = false;
        setFilter({ stats: [...availableStats.filter((as) => as.enabled)] });
      }}>Close & Apply</Button
    >
    <Button
      color="danger"
      onclick={() => {
        isOpen = false;
        resetRanges();
        setFilter({stats: []});
      }}>Reset</Button
    >
    <Button onclick={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
