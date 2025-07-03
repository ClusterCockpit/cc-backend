<!--
  @component Search Field for Job-Lists with separate mode if project filter is active

  Properties:
  - `presetProject String?`: Currently active project filter [Default: '']
  - `authlevel Number?`: The current users authentication level [Default: null]
  - `roles [Number]?`: Enum containing available roles [Default: null]
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
    setFilter
  } = $props();

  /* Const Init*/
  const throttle = 500;

  /* Var Init */
  let user = "";
  let project = presetProject ? presetProject : "";
  let jobName = "";
  let timeoutId = null;

  /* State Init */
  let mode = $state(presetProject ? "jobName" : "project");
  let term = $state("");

  /* Functions */
  function modeChanged() {
    if (mode == "user") {
      project = presetProject ? presetProject : "";
      jobName = "";
    } else if (mode == "project") {
      user = "";
      jobName = "";
    } else {
      project = presetProject ? presetProject : "";
      user = "";
    }
    termChanged(0);
  }

  // Compatibility: Handle "user role" and "no role" identically
  function termChanged(sleep = throttle) {
    if (roles && authlevel >= roles.manager) {
      if (mode == "user") user = term;
      else if (mode == "project") project = term;
      else jobName = term;

      if (timeoutId != null) clearTimeout(timeoutId);

      timeoutId = setTimeout(() => {
        setFilter({
          user,
          project,
          jobName
        });
      }, sleep);
    } else {
      if (mode == "project") project = term;
      else jobName = term;

      if (timeoutId != null) clearTimeout(timeoutId);

      timeoutId = setTimeout(() => {
        setFilter({
          project,
          jobName
        });
      }, sleep);
    }
  }

  function resetProject () {
    mode = "project"
    term = ""
    presetProject = ""
    project = ""
    jobName = ""
    user = ""
    termChanged(0);
  }
</script>

<InputGroup>
  <Input
    type="select"
    style="max-width: 120px;"
    class="form-select w-auto"
    title="Search Mode"
    bind:value={mode}
    onchange={modeChanged}
  >
    {#if !presetProject}
      <option value={"project"}>Project</option>
    {/if}
    {#if roles && authlevel >= roles.manager}
      <option value={"user"}>User</option>
    {/if}
    <option value={"jobName"}>Jobname</option>
  </Input>
  <Input
    type="text"
    bind:value={term}
    onchange={() => termChanged()}
    onkeyup={(event) => termChanged(event.key == "Enter" ? 0 : throttle)}
    placeholder={presetProject ? `Find in ${scrambleNames ? scramble(presetProject) : presetProject} ...` : `Find ${mode} ...`}
  />
  {#if presetProject}
  <Button title="Reset Project" onclick={() => resetProject()}
    ><Icon name="arrow-counterclockwise" /></Button
  >
  {/if}
</InputGroup>

