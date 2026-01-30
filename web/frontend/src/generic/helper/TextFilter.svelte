<!--
  @component Search Field for Job-Lists with separate mode if project filter is active

  Properties:
  - `presetProject String?`: Currently active project filter preset [Default: '']
  - `authlevel Number?`: The current users authentication level [Default: null]
  - `roles [Number]?`: Enum containing available roles [Default: null]
  - `filterBuffer [Obj]?`: Currently active filters, if any.
  - `setFilter Func`: The callback function to apply current filter selection
-->

<script>
  import { InputGroup, Input, Button, Icon } from "@sveltestrap/sveltestrap";
  import { scramble, scrambleNames } from "../utils.js";

  // Note: If page with this component has project preset, keep preset until reset
  /* Svelte 5 Props */
  let {
    presetProject = "",
    authlevel = null,
    roles = null,
    filterBuffer = [],
    setFilter
  } = $props();

  /* Const Init*/
  const throttle = 300;

  /* Var Init */
  let timeoutId = null;

  /* Derived */  
  const bufferProject = $derived.by(() => {
    let bp = filterBuffer.find((fb) => 
      Object.keys(fb).includes("project")
    )
    return bp?.project?.contains || null
  });

  const bufferUser = $derived.by(() => {
    let bu = filterBuffer.find((fb) => 
      Object.keys(fb).includes("user")
    )
    return bu?.user?.contains || null
  });

  const bufferJobName = $derived.by(() => {
    let bjn = filterBuffer.find((fb) => 
      Object.keys(fb).includes("jobName")
    )
    return bjn?.jobName?.contains || null
  });

  let mode = $derived.by(() => {
    if (presetProject) return "jobName" // Search by jobName if presetProject set
    else if (bufferUser) return "user"
    else if (bufferJobName) return "jobName"
    else return "project"
  });

  let term = $derived(bufferUser || bufferJobName || bufferProject || "");

  /* Functions */
  function inputChanged(sleep = throttle) {
    if (timeoutId != null) clearTimeout(timeoutId);
    if (mode == "user") {
      timeoutId = setTimeout(() => {
        setFilter({ user: term, project: (presetProject ? presetProject : null), jobName: null });
      }, sleep);    
    } else if (mode == "project") {
      timeoutId = setTimeout(() => {
        setFilter({ project: term, user: null, jobName: null });
      }, sleep);    
    } else if (mode == "jobName") {
      timeoutId = setTimeout(() => {
        setFilter({ jobName: term, user: null, project: (presetProject ? presetProject : null) });
      }, sleep);    
    }
  }

  function resetProject () {
    presetProject = "";
    term = "";
    inputChanged(0);
  }
</script>

<InputGroup>
  <Input
    type="select"
    style="max-width: 120px;"
    class="form-select w-auto"
    title="Search Mode"
    bind:value={mode}
    onchange={() => inputChanged()}
  >
    {#if !presetProject}
      <option value={"project"}>Project</option>
    {/if}
    {#if roles && authlevel >= roles?.manager}
      <option value={"user"}>User</option>
    {/if}
    <option value={"jobName"}>Jobname</option>
  </Input>
  <Input
    type="text"
    bind:value={term}
    onchange={() => inputChanged()}
    onkeyup={(event) => inputChanged(event.key == "Enter" ? 0 : throttle)}
    placeholder={presetProject ? `Find in ${scrambleNames ? scramble(presetProject) : presetProject} ...` : `Find ${mode} ...`}
  />
  {#if presetProject}
  <Button title="Reset Project" onclick={() => resetProject()}
    ><Icon name="arrow-counterclockwise" /></Button
  >
  {/if}
</InputGroup>

