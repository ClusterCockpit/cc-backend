<!--
  @component Displays job metaData, serves links to detail pages

  Properties:
  - `job Object`: The Job Object (GraphQL.Job)
  - `jobTags [Number]?`: The jobs tags as IDs, default useful for dynamically updating the tags [Default: job.tags]
  - `showJobSelect Bool?`: Show job selection interface for job comparison [Default: false]
  - `showTagEdit Bool?`: Show tag editing interface [Default: false]
  - `username String?`: The current username
  - `authlevel Number`: The current user authentication level
  - `roles [String]`: Available roles
  - `isSelected Bool`: Whether job is selected for comparison [Bindable, Default: false]
-->

<script>
  import { Badge, Button, Icon, Tooltip } from "@sveltestrap/sveltestrap";
  import { scrambleNames, scramble } from "../utils.js";
  import Tag from "../helper/Tag.svelte";
  import TagManagement from "../helper/TagManagement.svelte";

  /* Svelte 5 Props */
  let {
    job,
    jobTags = job.tags,
    showJobSelect = false,
    showTagEdit = false,
    username = null,
    authlevel = null,
    roles = null,
    isSelected = $bindable(false),
  } = $props();

  /* State Init */
  let displayCheck = $state(false);

  /* Functions */
  function formatDuration(duration) {
    const hours = Math.floor(duration / 3600);
    duration -= hours * 3600;
    const minutes = Math.floor(duration / 60);
    duration -= minutes * 60;
    const seconds = duration;
    return `${hours}:${("0" + minutes).slice(-2)}:${("0" + seconds).slice(-2)}`;
  }

  function getStateColor(state) {
    switch (state) {
      case "running":
        return "success";
      case "completed":
        return "primary";
      default:
        return "danger";
    }
  }

  function clipJobId(jid) {
    // Navigator clipboard api needs a secure context (https)
    if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard
          .writeText(jid)
          .catch((reason) => console.error(reason));
    } else {
      // Workaround: Create, Fill, And Copy Content of Textarea
      const textArea = document.createElement("textarea");
      textArea.value = jid;
      textArea.style.position = "absolute";
      textArea.style.left = "-999999px";
      document.body.prepend(textArea);
      textArea.select();
      try {
          document.execCommand('copy');
      } catch (error) {
          console.error(error);
      } finally {
          textArea.remove();
      }
    }
  }
</script>

<div>
  <p class="mb-2 text-truncate">
    <span class="d-flex justify-content-between">
      <span class="align-self-center fw-bold mr-2">
        <a href="/monitoring/job/{job.id}" target="_blank">{job.jobId}</a>
        ({job.cluster}) 
      </span>
      <span>
        {#if showJobSelect}
          <Button id={`${job.cluster}-${job.jobId}-select`} outline={!isSelected} color={isSelected? `success`: `secondary`} size="sm" class="mr-2"
            onclick={() => {
              isSelected = !isSelected
            }}>
            {#if isSelected}
              <Icon name="check-square"/>
            {:else }
              <Icon name="plus-square-dotted"/>
            {/if}
          </Button>
          <Tooltip
            target={`${job.cluster}-${job.jobId}-select`}
            placement="left">
              { 'Add or Remove Job to/from Comparison Selection' }
          </Tooltip>
        {/if}
        <Button id={`${job.cluster}-${job.jobId}-clipboard`} outline color="secondary" size="sm" onclick={() => {
          displayCheck = true;
          clipJobId(job.jobId);
          setTimeout(function () {
            displayCheck = false;
          }, 1000);
        }}>
          {#if displayCheck}
            <Icon name="clipboard2-check-fill"/>
          {:else}
            <Icon name="clipboard2"/>
          {/if}
        </Button>
        <Tooltip
          target={`${job.cluster}-${job.jobId}-clipboard`}
          placement="right">
            { displayCheck ? 'Copied!' : 'Copy Job ID to Clipboard' }
        </Tooltip>
      </span>
    </span>
    {#if job.metaData?.jobName}
      {#if job.metaData?.jobName.length <= 20}
        <span>{job.metaData.jobName}</span>
      {:else}
        <span
          style="cursor:help;"
          title={job.metaData.jobName}
        >
          {job.metaData.jobName}
  </span>
      {/if}
    {/if}
    {#if job.arrayJobId}
      Array Job: <a
        href="/monitoring/jobs/?arrayJobId={job.arrayJobId}&cluster={job.cluster}"
        target="_blank">#{job.arrayJobId}</a
      >
    {/if}
  </p>

  <p class="mb-2">
    <Icon name="person-fill" />
    <a class="fst-italic" href="/monitoring/user/{job.user}" target="_blank">
      {scrambleNames ? scramble(job.user) : job.user}
    </a>
    {#if job.userData && job.userData.name}
      ({scrambleNames ? scramble(job.userData.name) : job.userData.name})
    {/if}
    {#if job.project && job.project != "no project"}
      <br />
      <Icon name="people-fill" />
      <a
        class="fst-italic"
        href="/monitoring/jobs/?project={job.project}&projectMatch=eq"
        target="_blank"
      >
        {scrambleNames ? scramble(job.project) : job.project}
      </a>
    {/if}
  </p>

  <p class="mb-2">
    {#if job.numNodes == 1}
      {job.resources[0].hostname}
    {:else}
      {job.numNodes}
    {/if}
    <Icon name="pc-horizontal" />
    {#if job.shared != "none"}
      (shared)
    {/if}
    {#if job.numAcc > 0}
      , {job.numAcc} <Icon name="gpu-card" />
    {/if}
    {#if job.numHWThreads > 0}
      , {job.numHWThreads} <Icon name="cpu" />
    {/if}
    <br />
    {job.subCluster}
  </p>

  <p class="mb-2">
    Start: <span class="fw-bold"
      >{new Date(job.startTime).toLocaleString()}</span
    >
    <br />
    Duration: <span class="fw-bold">{formatDuration(job.duration)}</span>
    <Badge color={getStateColor(job.state)}>{job.state}</Badge>
    {#if job.walltime}
      <br />
      Walltime: <span class="fw-bold">{formatDuration(job.walltime)}</span>
    {/if}
  </p>

  {#if showTagEdit}
    <hr class="mt-0 mb-2"/>
    <p class="mb-1">
      <TagManagement bind:jobTags {job} {username} {authlevel} {roles} renderModal/> : 
      {#if jobTags?.length > 0}
        {#each jobTags as tag}
          <Tag {tag}/>
        {/each}
      {:else}
        <span style="font-size: 0.9rem; background-color: lightgray;" class="my-1 badge text-dark">No Tags</span>
      {/if}
    </p>
  {:else}
    <p class="mb-1">
      {#each jobTags as tag}
        <Tag {tag} />
      {/each}
    </p>
  {/if}
</div>
