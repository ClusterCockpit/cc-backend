<!--
    @component

    Properties:
    - job: GraphQL.Job
    - jobTags: Defaults to job.tags, usefull for dynamically updating the tags.
 -->
<script context="module">
    export const scrambleNames = window.localStorage.getItem("cc-scramble-names")
    export const scramble = (str) => [...str].reduce((x, c, i) => x * 7 + c.charCodeAt(0) * i * 21, 5).toString(32)
</script>
<script>
    import Tag from '../Tag.svelte';
    import { Badge, Icon } from 'sveltestrap';
    import { operationStore, query} from '@urql/svelte'

    export let job;
    export let jobTags = job.tags;
    export let showParallelJobs = false

    function formatDuration(duration) {
        const hours = Math.floor(duration / 3600);
        duration -= hours * 3600;
        const minutes = Math.floor(duration / 60);
        duration -= minutes * 60;
        const seconds = duration;
        return `${hours}:${('0' + minutes).slice(-2)}:${('0' + seconds).slice(-2)}`;
    }

    const filter = [
        {exclusive: job.exclusive},
        {sharedNode: {contains: job.resources[0].hostname }},
        {selfJobId: {neq: job.jobId }},
        {selfStartTime: job.startTime, selfDuration: job.duration}
    ]

    const parallelJobs = operationStore(`
    query($filter: [JobFilter!]!){
        jobsParallel(filter: $filter) {
            items { id, jobId }
            count
        }
    }`, {
        filter
    })

    if (showParallelJobs) { 
        query(parallelJobs)
    }
</script>

<div>
    <p>
        <span class="fw-bold"><a href="/monitoring/job/{job.id}" target="_blank">{job.jobId}</a> ({job.cluster})</span>
        {#if job.metaData?.jobName}
            <br/>
            {#if job.metaData?.jobName.length <= 25}
                <div>{job.metaData.jobName}</div>
            {:else}
                <div class="truncate" style="cursor:help; width:230px;" title={job.metaData.jobName}>{job.metaData.jobName}</div>
            {/if}
        {/if}
        {#if job.arrayJobId}
            Array Job: <a href="/monitoring/jobs/?arrayJobId={job.arrayJobId}&cluster={job.cluster}" target="_blank">#{job.arrayJobId}</a>
        {/if}
    </p>

    <p>
        <Icon name="person-fill"/>
        <a class="fst-italic" href="/monitoring/user/{job.user}" target="_blank">
            {scrambleNames ? scramble(job.user) : job.user}
        </a>
        {#if job.userData && job.userData.name}
            ({scrambleNames ? scramble(job.userData.name) : job.userData.name})
        {/if}
        {#if job.project && job.project != 'no project'}
            <br/>
            <Icon name="people-fill"/> 
            <a class="fst-italic" href="/monitoring/jobs/?project={job.project}&projectMatch=eq" target="_blank">
                {scrambleNames ? scramble(job.project) : job.project}
            </a>
        {/if}
        {#if showParallelJobs && $parallelJobs.data != null}
            <br/>
            <Icon name="diagram-3-fill"/> 
            {#if $parallelJobs.data.jobsParallel.count == 0}
                No Parallel Jobs
            {:else}
                {#each $parallelJobs.data.jobsParallel.items as pjob, index}
                    <a href="/monitoring/job/{pjob.id}" target="_blank">
                        {pjob.jobId}{#if index != $parallelJobs.data.jobsParallel.count - 1},{/if}
                    </a>
                {/each}
            {/if}
        {/if}
    </p>

    <p>
        {#if job.numNodes == 1}
            {job.resources[0].hostname}
        {:else}
            {job.numNodes}
        {/if}
        <Icon name="pc-horizontal"/>
        {#if job.exclusive != 1}
            (shared)
        {/if}
        {#if job.numAcc > 0}
            , {job.numAcc} <Icon name="gpu-card"/>
        {/if}
        {#if job.numHWThreads > 0}
            , {job.numHWThreads} <Icon name="cpu"/>
        {/if}
        <br/>
        {job.subCluster}
    </p>

    <p>
        Start: <span class="fw-bold">{(new Date(job.startTime)).toLocaleString()}</span>
        <br/>
        Duration: <span class="fw-bold">{formatDuration(job.duration)}</span>
        {#if job.state == 'running'}
            <Badge color="success">running</Badge>
        {:else if job.state != 'completed'}
            <Badge color="danger">{job.state}</Badge>
        {/if}
        {#if job.walltime}
            <br/>
            Walltime: <span class="fw-bold">{formatDuration(job.walltime)}</span>
        {/if}
    </p>

    <p>
        {#each jobTags as tag}
            <Tag tag={tag}/>
        {/each}
    </p>
</div>

<style>
    .truncate {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
</style>
