<!--
    @component Search Field for Job-Lists with separate mode if project filter is active

    Properties:
    - `presetProject String?`: Currently active project filter [Default: '']
    - `authlevel Number?`: The current users authentication level [Default: null]
    - `roles [Number]?`: Enum containing available roles [Default: null]

    Events:
    - `set-filter, {String?, String?, String?}`: Set 'user, project, jobName' filter in upstream component
 -->

<script>
  import { InputGroup, Input, Button, Icon } from "@sveltestrap/sveltestrap";
  import { createEventDispatcher } from "svelte";
  import { scramble, scrambleNames } from "../utils.js";

  const dispatch = createEventDispatcher();

  export let presetProject = ""; // If page with this component has project preset, keep preset until reset
  export let authlevel = null;
  export let roles = null;
  let mode = presetProject ? "jobName" : "project";
  let term = "";
  let user = "";
  let project = presetProject ? presetProject : "";
  let jobName = "";
  const throttle = 500;

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

  let timeoutId = null;
  // Compatibility: Handle "user role" and "no role" identically
  function termChanged(sleep = throttle) {
    if (roles && authlevel >= roles.manager) {
      if (mode == "user") user = term;
      else if (mode == "project") project = term;
      else jobName = term;

      if (timeoutId != null) clearTimeout(timeoutId);

      timeoutId = setTimeout(() => {
        dispatch("set-filter", {
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
        dispatch("set-filter", {
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
    termChanged(0);
  }
</script>

<InputGroup>
  <select
    style="max-width: 175px;"
    class="form-select"
    bind:value={mode}
    on:change={modeChanged}
  >
    {#if !presetProject}
      <option value={"project"}>Search Project</option>
    {/if}
    {#if roles && authlevel >= roles.manager}
      <option value={"user"}>Search User</option>
    {/if}
    <option value={"jobName"}>Search Jobname</option>
  </select>
  <Input
    type="text"
    bind:value={term}
    on:change={() => termChanged()}
    on:keyup={(event) => termChanged(event.key == "Enter" ? 0 : throttle)}
    placeholder={presetProject ? `Filter ${mode} in ${scrambleNames ? scramble(presetProject) : presetProject} ...` : `Filter ${mode} ...`}
  />
  {#if presetProject}
  <Button title="Reset Project" on:click={resetProject}
    ><Icon name="arrow-counterclockwise" /></Button
  >
  {/if}
</InputGroup>

