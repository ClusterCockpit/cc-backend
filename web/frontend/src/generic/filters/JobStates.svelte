<!--
  @component Filter sub-component for selecting job states

  Properties:
  - `isOpen Bool?`: Is this filter component opened [Bindable, Default: false]
  - `presetStates [String]?`: The latest selected filter state [Default: [...allJobStates]]
  - `presetShared String?`: The latest selected filter shared [Default: ""]
  - `presetShedule String?`: The latest selected filter schedule [Default: ""]
  - `setFilter Func`: The callback function to apply current filter selection

  Exported:
  - `const allJobStates [String]`: List of all available job states used in cc-backend
  - `const mapSharedStates {String:String}`: Object of all available shared states used in cc-backend with label
-->

<script module>
  export const allJobStates = [
    "pending",
    "running",
    "completed",
    "failed",
    "timeout",
    "deadline",
    "preempted",
    "suspended",
    "cancelled",
    "out_of_memory",
    "boot_fail",
    "node_fail"
  ];
  export const mapSharedStates = {
    none: "Exclusive",
    multi_user: "Shared",
    single_user: "Multitask",
  };
</script>

<script>
  /* Note: Ignore VSCode reported 'A component can only have one instance-level <script> element' error */
  
  import {
    Button,
    ListGroup,
    ListGroupItem,
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
    Input,
    Row,
    Col
  } from "@sveltestrap/sveltestrap";

  /* Svelte 5 Props */
  let {
    isOpen = $bindable(false),
    presetStates = [...allJobStates],
    presetShared = "",
    presetSchedule = "",
    setFilter
  } = $props();

  /* Const Init */
  const allSharedStates = [
    "none",
    "multi_user",
    "single_user",
  ];

  /* Derived */
  let pendingStates = $derived([...presetStates]);
  let pendingShared = $derived(presetShared);
  let pendingSchedule = $derived(presetSchedule);

</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Select Job States</ModalHeader>
  <ModalBody>
    <ListGroup>
      {#each allJobStates as state}
        <ListGroupItem>
          <input
            type="checkbox"
            bind:group={pendingStates}
            name="flavours"
            value={state}
          />
          {state.charAt(0).toUpperCase() + state.slice(1)}
        </ListGroupItem>
      {/each}
    </ListGroup>
    <hr/>
    <Row>
      <Col>
        <h5>Resource Sharing</h5>
        <Input type="radio" bind:group={pendingShared} value="" label="All" />
        {#each allSharedStates as shared}
          <Input type="radio" bind:group={pendingShared} value={shared} label={mapSharedStates[shared]} />
        {/each}
      </Col>
      <Col>
        <h5>Processing Type</h5>
        <Input type="radio" bind:group={pendingSchedule} value="" label="All" />
        <Input type="radio" bind:group={pendingSchedule} value="interactive" label="Interactive" />
        <Input type="radio" bind:group={pendingSchedule} value="batch" label="Batch Process" />
      </Col>
    </Row>
  </ModalBody>
  <ModalFooter>
    <Button
      color="primary"
      disabled={pendingStates.length == 0}
      onclick={() => {
        isOpen = false;
        setFilter({ states: [...pendingStates], shared: pendingShared, schedule: pendingSchedule });
      }}>Close & Apply</Button
    >
    {#if pendingStates.length != 0}
      <Button
        color="warning"
        onclick={() => {
          pendingStates = [];
        }}>Deselect All</Button
      >
    {:else}
      <Button
        color="success"
        onclick={() => {
          pendingStates = [...allJobStates];
        }}>Select All</Button
      >
    {/if}
    <Button
      color="danger"
      onclick={() => {
        isOpen = false;
        pendingStates = [...allJobStates];
        pendingShared = "";
        pendingSchedule = "";
        setFilter({ states: [...pendingStates], shared: pendingShared, schedule: pendingSchedule });
      }}>Reset</Button
    >
    <Button onclick={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
