<!--
    @component Concurrent Jobs Component; Lists all concurrent jobs in one scrollable card.

    Properties:
    - `cJobs JobLinkResultList`: List of concurrent Jobs
    - `showLinks Bool?`: Show list as clickable links [Default: false]
    - `displayTitle Bool?`: If to display cardHeader with title [Default: true]
    - `width String?`: Width of the card [Default: 'auto']
    - `height String?`: Height of the card [Default: '310px']
 -->

<script>
  import {
    Card,
    CardHeader,
    CardTitle,
    CardBody,
    Icon
  } from "@sveltestrap/sveltestrap";

  export let cJobs;
  export let showLinks = false;
  export let displayTitle = true;
  export let width = "auto";
  export let height = "310px";
</script>

<Card class="mt-1 overflow-auto" style="width: {width}; height: {height}">
  {#if displayTitle}
    <CardHeader>
      <CardTitle class="mb-0 d-flex justify-content-center">
        {cJobs.items.length} Concurrent Jobs
        <Icon
          style="cursor:help; margin-left:0.5rem;"
          name="info-circle"
          title="Jobs running on the same node with overlapping runtimes using shared resources"
        />
      </CardTitle>
    </CardHeader>
  {/if}
  <CardBody>
    {#if showLinks}
      <ul>
        <li>
          <a
            href="/monitoring/jobs/?{cJobs.listQuery}"
            target="_blank">See All</a
          >
        </li>
        {#each cJobs.items as cJob}
          <li>
            <a href="/monitoring/job/{cJob.id}" target="_blank"
              >{cJob.jobId}</a
            >
          </li>
        {/each}
      </ul>
    {:else}
      {#if displayTitle}
        <p>
          Jobs running on the same node with overlapping runtimes using shared resources.
        </p>
      {:else}
      <p>
        <b>{cJobs.items.length} </b>
        Jobs running on the same node with overlapping runtimes using shared resources.
      </p>
      {/if}
    {/if}
  </CardBody>
</Card>

<style>
  ul {
    columns: 2;
    -webkit-columns: 2;
    -moz-columns: 2;
  }
</style>
