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
        dispatch("update", { states });
      }}>Close & Apply</Button
    >
    <Button
      color="danger"
      on:click={() => {
        isOpen = false;
        states = [...allJobStates];
        pendingStates = [...allJobStates];
        dispatch("update", { states });
      }}>Reset</Button
    >
    <Button on:click={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
