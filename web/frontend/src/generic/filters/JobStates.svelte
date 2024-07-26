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

<script context="module">
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
  import { createEventDispatcher } from "svelte";
  import {
    Button,
    ListGroup,
    ListGroupItem,
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
  } from "@sveltestrap/sveltestrap";

  const dispatch = createEventDispatcher();

  export let isModified = false;
  export let isOpen = false;
  export let states = [...allJobStates];

  let pendingStates = [...states];
  $: isModified =
    states.length != pendingStates.length ||
    !states.every((state) => pendingStates.includes(state));
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
      on:click={() => {
        isOpen = false;
        states = [...pendingStates];
        dispatch("set-filter", { states });
      }}>Close & Apply</Button
    >
    <Button
      color="danger"
      on:click={() => {
        isOpen = false;
        states = [...allJobStates];
        pendingStates = [...allJobStates];
        dispatch("set-filter", { states });
      }}>Reset</Button
    >
    <Button on:click={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
