<script>
  import { createEventDispatcher, getContext } from "svelte";
  import {
    Button,
    ListGroup,
    ListGroupItem,
    Input,
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
    Icon,
  } from "@sveltestrap/sveltestrap";
  import { fuzzySearchTags } from "../utils.js";
  import Tag from "../Tag.svelte";

  const allTags = getContext("tags"),
    initialized = getContext("initialized"),
    dispatch = createEventDispatcher();

  export let isModified = false;
  export let isOpen = false;
  export let tags = [];

  let pendingTags = [...tags];
  $: isModified =
    tags.length != pendingTags.length ||
    !tags.every((tagId) => pendingTags.includes(tagId));

  let searchTerm = "";
</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Select Tags</ModalHeader>
  <ModalBody>
    <Input type="text" placeholder="Search" bind:value={searchTerm} />
    <br />
    <ListGroup>
      {#if $initialized}
        {#each fuzzySearchTags(searchTerm, allTags) as tag (tag)}
          <ListGroupItem>
            {#if pendingTags.includes(tag.id)}
              <Button
                outline
                color="danger"
                on:click={() =>
                  (pendingTags = pendingTags.filter((id) => id != tag.id))}
              >
                <Icon name="dash-circle" />
              </Button>
            {:else}
              <Button
                outline
                color="success"
                on:click={() => (pendingTags = [...pendingTags, tag.id])}
              >
                <Icon name="plus-circle" />
              </Button>
            {/if}

            <Tag {tag} />
          </ListGroupItem>
        {:else}
          <ListGroupItem disabled>No Tags</ListGroupItem>
        {/each}
      {/if}
    </ListGroup>
  </ModalBody>
  <ModalFooter>
    <Button
      color="primary"
      on:click={() => {
        isOpen = false;
        tags = [...pendingTags];
        dispatch("update", { tags });
      }}>Close & Apply</Button
    >
    <Button
      color="danger"
      on:click={() => {
        isOpen = false;
        tags = [];
        pendingTags = [];
        dispatch("update", { tags });
      }}>Reset</Button
    >
    <Button on:click={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
