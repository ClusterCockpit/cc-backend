<!--
    @component Concurrent Jobs Component; Lists all concurrent jobs in one scrollable card.

    Properties:
    - `cJobs JobLinkResultList`: List of concurrent Jobs
    - `showLinks Bool?`: Show list as clickable links [Default: false]
    - `renderCard Bool?`: If to render component as content only or with card wrapping [Default: true]
    - `width String?`: Width of the card [Default: 'auto']
    - `height String?`: Height of the card [Default: '310px']
 -->

<script>
  import {
    Card,
    CardHeader,
    CardBody,
    Icon
  } from "@sveltestrap/sveltestrap";

  export let cJobs;
  export let showLinks = false;
  export let renderCard = false;
  export let width = "auto";
  export let height = "400px";
</script>

{#if renderCard}
  <Card class="overflow-auto" style="width: {width}; height: {height}">
    <CardHeader class="mb-0 d-flex justify-content-center">
        {cJobs.items.length} Concurrent Jobs
        <Icon
          style="cursor:help; margin-left:0.5rem;"
          name="info-circle"
          title="Jobs running on the same node with overlapping runtimes using shared resources"
        />
    </CardHeader>
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
        <ul>
          {#each cJobs.items as cJob}
            <li>
              {cJob.jobId}
            </li>
          {/each}
        </ul>
      {/if}
    </CardBody>
  </Card>
{:else}
  <p>
    {cJobs.items.length} Jobs running on the same node with overlapping runtimes using shared resources. 
    ( <a
      href="/monitoring/jobs/?{cJobs.listQuery}"
      target="_blank">See All</a
    > )
  </p>
  <hr/>
  {#if showLinks}
    <ul>
      {#each cJobs.items as cJob}
        <li>
          <a href="/monitoring/job/{cJob.id}" target="_blank"
            >{cJob.jobId}</a
          >
        </li>
      {/each}
    </ul>
  {:else}
    <ul>
      {#each cJobs.items as cJob}
        <li>
          {cJob.jobId}
        </li>
      {/each}
    </ul>
  {/if}
{/if}

<style>
  ul {
    columns: 3;
    -webkit-columns: 3;
    -moz-columns: 3;
  }
</style>
