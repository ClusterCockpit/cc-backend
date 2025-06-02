<!--
    @component Filter sub-component for selecting job states

    Properties:
    - `isModified Bool?`: Is this filter component modified [Default: false]
    - `isOpen Bool?`: Is this filter component opened [Default: false]
    - `states [String]?`: The currently selected states [Default: [...allJobStates]]

    Events:
    - `set-filter, {[String]}`: Set 'states' filter in upstream component

    Exported:
    - `const allJobStates [String]`: List of all available job states used in cc-backend
 -->

<script module>
  export const allJobStates = [
    "running",
    "completed",
    "failed",
    "cancelled",
    "stopped",
    "timeout",
    "preempted",
    "out_of_memory",
  ];
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
  } from "@sveltestrap/sveltestrap";

  /* Svelte 5 Props */
  let {
    isOpen = $bindable(false),
    presetStates = [...allJobStates],
    setFilter
  } = $props();

  /* State Init */
  let pendingStates = $state([...presetStates]);

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
          {state}
        </ListGroupItem>
      {/each}
    </ListGroup>
  </ModalBody>
  <ModalFooter>
    <Button
      color="primary"
      disabled={pendingStates.length == 0}
      onclick={() => {
        isOpen = false;
        setFilter({ states: [...pendingStates] });
      }}>Close & Apply</Button
    >
    <Button
      color="warning"
      onclick={() => {
        pendingStates = [];
      }}>Deselect All</Button
    >
    <Button
      color="danger"
      onclick={() => {
        isOpen = false;
        pendingStates = [...allJobStates];
        setFilter({ states: [...pendingStates] });
      }}>Reset</Button
    >
    <Button onclick={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
