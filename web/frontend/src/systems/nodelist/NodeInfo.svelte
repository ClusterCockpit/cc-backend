<!--
    @component Displays job metaData, serves links to detail pages

    Properties:
    - `job Object`: The Job Object (GraphQL.Job)
    - `jobTags [Number]?`: The jobs tags as IDs, default useful for dynamically updating the tags [Default: job.tags]
 -->

<script>
  import { Badge, Icon } from "@sveltestrap/sveltestrap";
  import { scrambleNames, scramble } from "../../generic/utils.js";
  import Tag from "../../generic/helper/Tag.svelte";
  import TagManagement from "../../generic/helper/TagManagement.svelte";

  export let job;
  export let jobTags = job.tags;
  export let showTagedit = false;
  export let username = null;
  export let authlevel= null;
  export let roles = null;

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
</script>

<div>
  <p class="mb-2">
    <span class="fw-bold"
      ><a href="/monitoring/job/{job.id}" target="_blank">{job.jobId}</a>
      ({job.cluster})</span
    >
    {#if job.metaData?.jobName}
      <br />
      {#if job.metaData?.jobName.length <= 25}
        <div>{job.metaData.jobName}</div>
      {:else}
        <div
          class="truncate"
          style="cursor:help; width:230px;"
          title={job.metaData.jobName}
        >
          {job.metaData.jobName}
        </div>
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
    {#if job.exclusive != 1}
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

  {#if showTagedit}
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

<style>
  .truncate {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
