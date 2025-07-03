<!--
  @component Filter sub-component for selecting job energies

  Properties:
  - `isOpen Bool?`: Is this filter component opened [Bindable, efault: false]
  - `presetEnergy Object?`: Object containing the latest energy filter parameters
    - Default: { from: null, to: null }
  - `setFilter Func`: The callback function to apply current filter selection
-->

<script>
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
    isOpen = $bindable(false),
    presetEnergy = {
      from: null,
      to: null
    },
    setFilter,
  } = $props();

  /* State Init */
  let energyState = $state(presetEnergy);
</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Filter based on energy</ModalHeader>
  <ModalBody>
    <div class="mb-3">
      <div class="mb-0"><b>Total Job Energy (kWh)</b></div>
      <DoubleRangeSlider
        changeRange={(detail) => {
          energyState.from = detail[0];
          energyState.to = detail[1];
        }}
        sliderMin={0.0}
        sliderMax={1000.0}
        fromPreset={energyState?.from? energyState.from : 0.0}
        toPreset={energyState?.to? energyState.to : 1000.0}
      />
    </div>
  </ModalBody>
  <ModalFooter>
    <Button
      color="primary"
      onclick={() => {
        isOpen = false;
        setFilter({ energy: energyState });
      }}>Close & Apply</Button
    >
    <Button
      color="danger"
      onclick={() => {
        isOpen = false;
        energyState = {from: null, to: null};
        setFilter({ energy: energyState });
      }}>Reset</Button
    >
    <Button onclick={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
