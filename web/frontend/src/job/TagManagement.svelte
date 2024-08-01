<!--
    @component Job View Subcomponent; allows management of job tags by deletion or new entries

    Properties:
    - `job Object`: The job object
    - `jobTags [Number]`: The array of currently designated tags
    - `username String`: Empty string if auth. is disabled, otherwise the username as string
    - `authlevel Number`: The current users authentication level
    - `roles [Number]`: Enum containing available roles
 -->
<script>
  import { getContext } from "svelte";
  import { gql, getContextClient, mutationStore } from "@urql/svelte";
  import {
    Icon,
    Button,
    ListGroupItem,
    Spinner,
    Modal,
    Input,
    ModalBody,
    ModalHeader,
    ModalFooter,
    Alert,
  } from "@sveltestrap/sveltestrap";
  import { fuzzySearchTags } from "../generic/utils.js";
  import Tag from "../generic/helper/Tag.svelte";

  export let job;
  export let jobTags = job.tags;
  export let username;
  export let authlevel;
  export let roles;

  let allTags = getContext("tags"),
    initialized = getContext("initialized");
  let newTagType = "",
    newTagName = "",
    newTagScope = username;
  let filterTerm = "";
  let pendingChange = false;
  let isOpen = false;

  const client = getContextClient();

  const createTagMutation = ({ type, name, scope }) => {
    return mutationStore({
      client: client,
      query: gql`
        mutation ($type: String!, $name: String!, $scope: String!) {
          createTag(type: $type, name: $name, scope: $scope) {
            id
            type
            name
            scope
          }
        }
      `,
      variables: { type, name, scope },
    });
  };

  const addTagsToJobMutation = ({ job, tagIds }) => {
    return mutationStore({
      client: client,
      query: gql`
        mutation ($job: ID!, $tagIds: [ID!]!) {
          addTagsToJob(job: $job, tagIds: $tagIds) {
            id
            type
            name
            scope
          }
        }
      `,
      variables: { job, tagIds },
    });
  };

  const removeTagsFromJobMutation = ({ job, tagIds }) => {
    return mutationStore({
      client: client,
      query: gql`
        mutation ($job: ID!, $tagIds: [ID!]!) {
          removeTagsFromJob(job: $job, tagIds: $tagIds) {
            id
            type
            name
            scope
          }
        }
      `,
      variables: { job, tagIds },
    });
  };

  let allTagsFiltered; // $initialized is in there because when it becomes true, allTags is initailzed.
  $: allTagsFiltered = ($initialized, fuzzySearchTags(filterTerm, allTags));

  $: {
    newTagType = "";
    newTagName = "";
    let parts = filterTerm.split(":").map((s) => s.trim());
    if (parts.length == 2 && parts.every((s) => s.length > 0)) {
      newTagType = parts[0];
      newTagName = parts[1];
    }
  }

  function isNewTag(type, name) {
    for (let tag of allTagsFiltered)
      if (tag.type == type && tag.name == name) return false;
    return true;
  }

  function createTag(type, name, scope) {
    pendingChange = true;
    createTagMutation({ type: type, name: name, scope: scope }).subscribe((res) => {
      if (res.fetching === false && !res.error) {
        pendingChange = false;
        allTags = [...allTags, res.data.createTag];
        newTagType = "";
        newTagName = "";
        addTagToJob(res.data.createTag);
      } else if (res.fetching === false && res.error) {
        throw res.error;
      }
    });
  }

  function addTagToJob(tag) {
    pendingChange = tag.id;
    addTagsToJobMutation({ job: job.id, tagIds: [tag.id] }).subscribe((res) => {
      if (res.fetching === false && !res.error) {
        jobTags = job.tags = res.data.addTagsToJob;
        pendingChange = false;
      } else if (res.fetching === false && res.error) {
        throw res.error;
      }
    });
  }

  function removeTagFromJob(tag) {
    pendingChange = tag.id;
    removeTagsFromJobMutation({ job: job.id, tagIds: [tag.id] }).subscribe(
      (res) => {
        if (res.fetching === false && !res.error) {
          jobTags = job.tags = res.data.removeTagsFromJob;
          pendingChange = false;
        } else if (res.fetching === false && res.error) {
          throw res.error;
        }
      },
    );
  }
</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>
    Manage Tags
    {#if pendingChange !== false}
      <Spinner size="sm" secondary />
    {:else}
      <Icon name="tags" />
    {/if}
  </ModalHeader>
  <ModalBody>
    <Input
      style="width: 100%;"
      type="text"
      placeholder="Search Tags"
      bind:value={filterTerm}
    />

    <br />

    <Alert color="info">
      Search using "<code>type: name</code>". If no tag matches your search, a
      button for creating a new one will appear.
    </Alert>

    <ul class="list-group">
      {#each allTagsFiltered as tag}
        <ListGroupItem>
          <Tag {tag} />

          <span style="float: right;">
            {#if pendingChange === tag.id}
              <Spinner size="sm" secondary />
            {:else if job.tags.find((t) => t.id == tag.id)}
              <Button
                size="sm"
                outline
                color="danger"
                on:click={() => removeTagFromJob(tag)}
              >
                <Icon name="x" />
              </Button>
            {:else}
              <Button
                size="sm"
                outline
                color="success"
                on:click={() => addTagToJob(tag)}
              >
                <Icon name="plus" />
              </Button>
            {/if}
          </span>
        </ListGroupItem>
      {:else}
        <ListGroupItem disabled>
          <i>No tags matching</i>
        </ListGroupItem>
      {/each}
    </ul>
    <br />
    {#if newTagType && newTagName && isNewTag(newTagType, newTagName)}
      <div class="d-flex">
        <Button
          style="margin-right: 10px;"
          outline
          color="success"
          on:click={(e) => (
            e.preventDefault(), createTag(newTagType, newTagName, newTagScope)
          )}
        >
          Create & Add Tag:
          <Tag tag={{ type: newTagType, name: newTagName, scope: newTagScope }} clickable={false}/>
        </Button>
        {#if roles && authlevel >= roles.admin}
          <select
            style="max-width: 175px;"
            class="form-select"
            bind:value={newTagScope}
          >
            <option value={username}>Scope: Private</option>
            <option value={"global"}>Scope: Global</option>
            <option value={"admin"}>Scope: Admin</option>
          </select>
        {/if}
      </div>
    {:else if allTagsFiltered.length == 0}
      <Alert>Search Term is not a valid Tag (<code>type: name</code>)</Alert>
    {/if}
  </ModalBody>
  <ModalFooter>
    <Button color="primary" on:click={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>

<Button outline on:click={() => (isOpen = true)}>
  Manage Tags <Icon name="tags" />
</Button>

<style>
  ul.list-group {
    max-height: 450px;
    margin-bottom: 10px;
    overflow: scroll;
  }
</style>
