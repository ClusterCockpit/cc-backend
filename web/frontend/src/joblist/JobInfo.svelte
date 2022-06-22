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

    export let job;
    export let jobTags = job.tags;

    function formatDuration(duration) {
        const hours = Math.floor(duration / 3600);
        duration -= hours * 3600;
        const minutes = Math.floor(duration / 60);
        duration -= minutes * 60;
        const seconds = duration;
        return `${hours}:${('0' + minutes).slice(-2)}:${('0' + seconds).slice(-2)}`;
    }
</script>

<div>
    <p>
        <span class="fw-bold"><a href="/monitoring/job/{job.id}" target="_blank">{job.jobId}</a> ({job.cluster})</span>
        {#if job.metaData?.jobName}
            <br/>
            {job.metaData.jobName}
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
            <Icon name="people-fill"/> {job.project}
        {/if}
    </p>

    <p>
        {job.numNodes} <Icon name="pc-horizontal"/>
        {#if job.exclusive != 1}
            (shared)
        {/if}
        {#if job.numAcc > 0}
            , {job.numAcc} <Icon name="gpu-card"/>
        {/if}
        {#if job.numHWThreads > 0}
            , {job.numHWThreads} <Icon name="cpu"/>
        {/if}
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