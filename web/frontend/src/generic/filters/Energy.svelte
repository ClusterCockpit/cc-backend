<!--
    @component Filter sub-component for selecting job energies

    Properties:
    - `isOpen Bool?`: Is this filter component opened [Default: false]
    - `energy Object?`: The currently selected total energy filter [Default: {from:null, to:null}]

    Events:
    - `set-filter, {Object}`: Set 'energy' filter in upstream component
 -->

<script>
  import { createEventDispatcher } from "svelte";
  import {
    Button,
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
  } from "@sveltestrap/sveltestrap";
  import DoubleRangeSlider from "../select/DoubleRangeSlider.svelte";

  const dispatch = createEventDispatcher();

  const energyMaximum = 1000.0;

  export let isOpen = false;
  export let energy= {from: null, to: null};

  function resetRanges() {
      energy.from = null
      energy.to = null
  }
</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Filter based on energy</ModalHeader>
  <ModalBody>
    <h4>Total Job Energy (kWh)</h4>
    <DoubleRangeSlider
      on:change={({ detail }) => (
        (energy.from = detail[0]), (energy.to = detail[1])
      )}
      min={0.0}
      max={energyMaximum}
      firstSlider={energy?.from ? energy.from : 0.0}
      secondSlider={energy?.to ? energy.to : energyMaximum}
      inputFieldFrom={energy?.from ? energy.from : null}
      inputFieldTo={energy?.to ? energy.to : null}
    />
  </ModalBody>
  <ModalFooter>
    <Button
      color="primary"
      on:click={() => {
        isOpen = false;
        dispatch("set-filter", { energy });
      }}>Close & Apply</Button
    >
    <Button
      color="danger"
      on:click={() => {
        isOpen = false;
        resetRanges();
        dispatch("set-filter", { energy });
      }}>Reset</Button
    >
    <Button on:click={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
