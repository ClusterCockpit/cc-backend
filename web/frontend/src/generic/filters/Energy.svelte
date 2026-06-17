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
    Tooltip,
    Icon
  } from "@sveltestrap/sveltestrap";
  import DoubleRangeSlider from "../select/DoubleRangeSlider.svelte";

  /* Svelte 5 Props */
  let {
    isOpen = $bindable(false),
    presetEnergy = { from: null, to: null },
    setFilter,
  } = $props();

  /* Const */
  const minEnergyPreset = 0;
  const maxEnergyPreset = 100;

  /* Derived */
  // Pending
  let pendingEnergyState = $derived({
    from: presetEnergy?.from || minEnergyPreset,
    to: (presetEnergy.to == 0) ? null : presetEnergy.to,
  });
  // Changable
  let energyState = $derived({
    from: presetEnergy?.from || minEnergyPreset,
    to: (presetEnergy.to == 0) ? null : presetEnergy.to,
  });

  const energyActive = $derived(!(JSON.stringify(energyState) === JSON.stringify({ from: minEnergyPreset, to: null })));

  /* Function */
  function setEnergy() {
    if (energyActive) {
      pendingEnergyState = {
        from: (!energyState?.from) ? 0 : energyState.from,
        to: (energyState.to === null) ? 0 : energyState.to
      };
    } else {
      pendingEnergyState = { from: null, to: null};
    };
  }
</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Filter based on energy</ModalHeader>
  <ModalBody>
    <div class="mb-3">
      <div class="mb-0">
        <b>Total Job Energy (kWh)</b>
        <Icon id="energy-info" style="cursor:help; padding-right: 10px;" size="sm" name="info-circle"/>
      </div>
      <Tooltip target={`energy-info`} placement="right">
        Generalized Presets. Use input fields to change to higher values.
      </Tooltip>
      <DoubleRangeSlider
        changeRange={(detail) => {
          energyState.from = detail[0];
          energyState.to = detail[1];
        }}
        sliderMin={minEnergyPreset}
        sliderMax={maxEnergyPreset}
        fromPreset={energyState.from}
        toPreset={energyState.to}
      />
    </div>
  </ModalBody>
  <ModalFooter>
    <Button
      color="primary"
      onclick={() => {
        isOpen = false;
        setEnergy();
        setFilter({ energy: pendingEnergyState });
      }}>Close & Apply</Button
    >
    <Button
      color="danger"
      onclick={() => {
        isOpen = false;
        pendingEnergyState = {from: null, to: null};
        setFilter({ energy: pendingEnergyState });
      }}>Reset</Button
    >
    <Button onclick={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
