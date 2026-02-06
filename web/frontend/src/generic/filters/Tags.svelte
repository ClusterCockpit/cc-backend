<!--
  @component Filter sub-component for selecting tags

  Properties:
  - `isOpen Bool?`: Is this filter component opened [Bindable, Default: false]
  - `presetTags [Number]?`: The currently selected tags (as IDs) [Default: []]
  - `setFilter Func`: The callback function to apply current filter selection
-->

<script>
  import { getContext } from "svelte";
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
  import Tag from "../helper/Tag.svelte";

  /* Svelte 5 Props */
  let {
    isOpen = $bindable(false),
    presetTags = [],
    setFilter
  } = $props();

  /* Derived */
  const initialized = $derived(getContext("initialized") || false)
  const allTags = $derived($initialized ? getContext("tags") : [])

  /* State Init */
  let searchTerm = $state("");

  /* Derived */
  let pendingTags = $derived(presetTags);

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
                onclick={() =>
                  (pendingTags = pendingTags.filter((id) => id != tag.id))}
              >
                <Icon name="dash-circle" />
              </Button>
            {:else}
              <Button
                outline
                color="success"
                onclick={() => (pendingTags = [...pendingTags, tag.id])}
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
      onclick={() => {
        isOpen = false;
        setFilter({ tags: [...pendingTags] });
      }}>Close & Apply</Button
    >
    <Button
      color="warning"
      onclick={() => {
        pendingTags = [];
      }}>Clear Selection</Button
    >
    <Button
      color="danger"
      onclick={() => {
        isOpen = false;
        pendingTags = [];
        setFilter({ tags: [...pendingTags] });
      }}>Reset</Button
    >
    <Button onclick={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
