<!--
    @component Job Info Subcomponent; allows management of job tags by deletion or new entries

    Properties:
    - `job Object`: The job object
    - `jobTags [Number]`: The array of currently designated tags
    - `username String`: Empty string if auth. is disabled, otherwise the username as string
    - `authlevel Number`: The current users authentication level
    - `roles [Number]`: Enum containing available roles
    - `renderModal Bool?`: If component is rendered as bootstrap modal button [Default: true]
 -->
<script>
  import { getContext } from "svelte";
  import { gql, getContextClient, mutationStore } from "@urql/svelte";
  import {
    Row,
    Col,
    Icon,
    Button,
    ListGroup,
    ListGroupItem,
    Input,
    InputGroup,
    InputGroupText,
    Spinner,
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
    Alert,
    Tooltip,
  } from "@sveltestrap/sveltestrap";
  import { fuzzySearchTags } from "../utils.js";
  import Tag from "./Tag.svelte";

  export let job;
  export let jobTags = job.tags;
  export let username;
  export let authlevel;
  export let roles;
  export let renderModal = true;

  let allTags = getContext("tags"),
    initialized = getContext("initialized");
  let newTagType = "",
    newTagName = "",
    newTagScope = username;
  let filterTerm = "";
  let pendingChange = false;
  let isOpen = false;
  const isAdmin = (roles && authlevel >= roles.admin);

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

  $: allTagsFiltered = ($initialized, fuzzySearchTags(filterTerm, allTags));
  $: usedTagsFiltered = matchJobTags(jobTags, allTagsFiltered, 'used', isAdmin);
  $: unusedTagsFiltered = matchJobTags(jobTags, allTagsFiltered, 'unused', isAdmin);

  $: {
    newTagType = "";
    newTagName = "";
    let parts = filterTerm.split(":").map((s) => s.trim());
    if (parts.length == 2 && parts.every((s) => s.length > 0)) {
      newTagType = parts[0];
      newTagName = parts[1];
    }
  }

  function matchJobTags(tags, availableTags, type, isAdmin) {
    const jobTagIds = tags.map((t) => t.id)
    if (type == 'used') {
      return availableTags.filter((at) => jobTagIds.includes(at.id))
    } else if (type == 'unused' && isAdmin) {
      return availableTags.filter((at) => !jobTagIds.includes(at.id))
    } else if (type == 'unused' && !isAdmin) { // Normal Users should not see unused global tags here
      return availableTags.filter((at) => !jobTagIds.includes(at.id) && at.scope !== "global")
    }
    return []
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

{#if renderModal}
  <Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
    <ModalHeader>
      Manage Tags <Icon name="tags"/>
    </ModalHeader>
    <ModalBody>
      <InputGroup class="mb-3">
        <Input
          type="text"
          placeholder="Search Tags"
          bind:value={filterTerm}
        />
        <InputGroupText id={`tag-management-info-modal`} style="cursor:help; font-size:larger;align-content:center;">
          <Icon name=info-circle/>
        </InputGroupText>
        <Tooltip
          target={`tag-management-info-modal`}
          placement="right">
            Search using "type: name". If no tag matches your search, a
            button for creating a new one will appear.
        </Tooltip>
      </InputGroup>
    
      <div class="mb-3 scroll-group">
        {#if usedTagsFiltered.length > 0}
          <ListGroup class="mb-3">
            {#each usedTagsFiltered as utag}
              <ListGroupItem color="light">
                <Tag tag={utag} />
      
                <span style="float: right;">
                  {#if pendingChange === utag.id}
                    <Spinner size="sm" secondary />
                  {:else}
                    <Button
                      size="sm"
                      color="dark"
                      outline
                      disabled
                    >
                      {#if utag.scope === 'global' || utag.scope === 'admin'}
                        {utag.scope.charAt(0).toUpperCase() + utag.scope.slice(1)} Tag
                      {:else}
                        Private Tag
                      {/if}
                    </Button>
                    {#if isAdmin || (utag.scope !== 'global' && utag.scope !== 'admin')}
                      <Button
                        size="sm"
                        color="danger"
                        on:click={() => removeTagFromJob(utag)}
                      >
                        <Icon name="x" />
                      </Button>
                    {/if}
                  {/if}
                </span>
              </ListGroupItem>
            {/each}
          </ListGroup>
        {:else if filterTerm !== ""}
          <ListGroup class="mb-3">
            <ListGroupItem disabled>
              <i>No attached tags matching.</i>
            </ListGroupItem>
          </ListGroup>
        {:else}
          <ListGroup class="mb-3">
            <ListGroupItem disabled>
              <i>Job has no attached tags.</i>
            </ListGroupItem>
          </ListGroup>
        {/if}
      
        {#if unusedTagsFiltered.length > 0}
          <ListGroup class="">
            {#each unusedTagsFiltered as uutag}
              <ListGroupItem color="secondary">
                <Tag tag={uutag} />
      
                <span style="float: right;">
                  {#if pendingChange === uutag.id}
                    <Spinner size="sm" secondary />
                  {:else}
                    <Button
                      size="sm"
                      color="dark"
                      outline
                      disabled
                    >
                      {#if uutag.scope === 'global' || uutag.scope === 'admin'}
                        {uutag.scope.charAt(0).toUpperCase() + uutag.scope.slice(1)} Tag
                      {:else}
                        Private Tag
                      {/if}
                    </Button>
                    {#if isAdmin || (uutag.scope !== 'global' && uutag.scope !== 'admin')}
                      <Button
                        size="sm"
                        color="success"
                        on:click={() => addTagToJob(uutag)}
                      >
                        <Icon name="plus" />
                      </Button>
                    {/if}
                  {/if}
                </span>
              </ListGroupItem>
            {/each}
          </ListGroup>
        {:else if filterTerm !== ""}
          <ListGroup class="">
            <ListGroupItem disabled>
              <i>No unused tags matching.</i>
            </ListGroupItem>
          </ListGroup>
        {:else}
          <ListGroup class="">
            <ListGroupItem disabled>
              <i>No unused tags available.</i>
            </ListGroupItem>
          </ListGroup>
        {/if}
      </div>
    
      {#if newTagType && newTagName && isNewTag(newTagType, newTagName)}
        <Row>
          <Col xs={isAdmin ? 7 : 12} md={12} lg={isAdmin ? 7 : 12} xl={12} xxl={isAdmin ? 7 : 12} class="mb-2">
            <Button
              outline
              style="width:100%;"
              color="success"
              on:click={(e) => (
                e.preventDefault(), createTag(newTagType, newTagName, newTagScope)
              )}
            >
              Add new tag:
              <Tag tag={{ type: newTagType, name: newTagName, scope: newTagScope }} clickable={false}/>
            </Button>
          </Col>
          {#if isAdmin}
            <Col xs={5} md={12} lg={5} xl={12} xxl={5} class="mb-2" style="align-content:center;">
              <Input type="select" bind:value={newTagScope}>
                <option value={username}>Scope: Private</option>
                <option value={"global"}>Scope: Global</option>
                <option value={"admin"}>Scope: Admin</option>
              </Input>
            </Col>
          {/if}
        </Row>
      {:else if filterTerm !== "" && allTagsFiltered.length == 0}
        <Alert color="info">
          Search Term is not a valid Tag (<code>type: name</code>)
        </Alert>
      {:else if filterTerm == "" && unusedTagsFiltered.length == 0}
        <Alert color="info">
          Type "<code>type: name</code>" into the search field to create a new tag.
        </Alert>
      {/if}
    </ModalBody>
    <ModalFooter>
      <Button color="primary" on:click={() => (isOpen = false)}>Close</Button>
    </ModalFooter>
  </Modal>

  <Button outline on:click={() => (isOpen = true)} size="sm" color="primary">
    Manage {jobTags?.length ? jobTags.length : ''} Tags
  </Button>

{:else}

  <InputGroup class="mb-3">
    <Input
      type="text"
      placeholder="Search Tags"
      bind:value={filterTerm}
    />
    <InputGroupText id={`tag-management-info`} style="cursor:help; font-size:larger;align-content:center;">
      <Icon name=info-circle/>
    </InputGroupText>
    <Tooltip
      target={`tag-management-info`}
      placement="right">
        Search using "type: name". If no tag matches your search, a
        button for creating a new one will appear.
    </Tooltip>
  </InputGroup>

  {#if usedTagsFiltered.length > 0}
    <ListGroup class="mb-3">
      {#each usedTagsFiltered as utag}
        <ListGroupItem color="light">
          <Tag tag={utag} />

          <span style="float: right;">
            {#if pendingChange === utag.id}
              <Spinner size="sm" secondary />
            {:else}
              {#if utag.scope === 'global' || utag.scope === 'admin'}
                {#if isAdmin}
                  <Button
                    size="sm"
                    color="danger"
                    on:click={() => removeTagFromJob(utag)}
                  >
                    <Icon name="x" />
                  </Button>
                {/if}
              {:else}
                <Button
                  size="sm"
                  color="danger"
                  on:click={() => removeTagFromJob(utag)}
                >
                  <Icon name="x" />
                </Button>
              {/if}
            {/if}
          </span>
        </ListGroupItem>
      {/each}
    </ListGroup>
  {:else if filterTerm !== ""}
    <ListGroup class="mb-3">
      <ListGroupItem disabled>
        <i>No attached tags matching.</i>
      </ListGroupItem>
    </ListGroup>
  {:else}
    <ListGroup class="mb-3">
      <ListGroupItem disabled>
        <i>Job has no attached tags.</i>
      </ListGroupItem>
    </ListGroup>
  {/if}

  {#if unusedTagsFiltered.length > 0}
    <ListGroup class="mb-3">
      {#each unusedTagsFiltered as uutag}
        <ListGroupItem color="secondary">
          <Tag tag={uutag} />

          <span style="float: right;">
            {#if pendingChange === uutag.id}
              <Spinner size="sm" secondary />
            {:else}
              {#if uutag.scope === 'global' || uutag.scope === 'admin'}
                {#if isAdmin}
                  <Button
                    size="sm"
                    color="success"
                    on:click={() => addTagToJob(uutag)}
                  >
                    <Icon name="plus" />
                  </Button>
                {/if}
              {:else}
                <Button
                  size="sm"
                  color="success"
                  on:click={() => addTagToJob(uutag)}
                >
                  <Icon name="plus" />
                </Button>
              {/if}
            {/if}
          </span>
        </ListGroupItem>
      {/each}
    </ListGroup>
  {:else if filterTerm !== ""}
    <ListGroup class="mb-3">
      <ListGroupItem disabled>
        <i>No unused tags matching.</i>
      </ListGroupItem>
    </ListGroup>
  {:else}
    <ListGroup class="mb-3">
      <ListGroupItem disabled>
        <i>No unused tags available.</i>
      </ListGroupItem>
    </ListGroup>
  {/if}

  {#if newTagType && newTagName && isNewTag(newTagType, newTagName)}
    <Row>
      <Col xs={isAdmin ? 7 : 12} md={12} lg={isAdmin ? 7 : 12} xl={12} xxl={isAdmin ? 7 : 12} class="mb-2">
        <Button
          outline
          style="width:100%;"
          color="success"
          on:click={(e) => (
            e.preventDefault(), createTag(newTagType, newTagName, newTagScope)
          )}
        >
          Add new tag:
          <Tag tag={{ type: newTagType, name: newTagName, scope: newTagScope }} clickable={false}/>
        </Button>
      </Col>
      {#if isAdmin}
        <Col xs={5} md={12} lg={5} xl={12} xxl={5} class="mb-2" style="align-content:center;">
          <Input type="select" bind:value={newTagScope}>
            <option value={username}>Scope: Private</option>
            <option value={"global"}>Scope: Global</option>
            <option value={"admin"}>Scope: Admin</option>
          </Input>
        </Col>
      {/if}
    </Row>
  {:else if filterTerm !== "" && allTagsFiltered.length == 0}
    <Alert color="info">
      Search Term is not a valid Tag (<code>type: name</code>)
    </Alert>
  {:else if filterTerm == "" && unusedTagsFiltered.length == 0}
    <Alert color="info">
      Type "<code>type: name</code>" into the search field to create a new tag.
    </Alert>
  {/if}
{/if}

<style>
  .scroll-group {
    max-height: 500px;
    overflow-y: auto;
  }
</style>
