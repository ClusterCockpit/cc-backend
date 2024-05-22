<script>
  import { InputGroup, Input } from "@sveltestrap/sveltestrap";
  import { createEventDispatcher } from "svelte";

  const dispatch = createEventDispatcher();

  export let authlevel;
  export let roles;
  let mode = "user";
  let term = "";
  let user = "";
  let project = "";
  let jobName = "";
  const throttle = 500;

  function modeChanged() {
    if (mode == "user") {
      project = term;
      term = user;
    } else if (mode == "project") {
      user = term;
      term = project;
    } else {
      jobName = term;
      term = jobName;
    }
    termChanged(0);
  }

  let timeoutId = null;
  // Compatibility: Handle "user role" and "no role" identically
  function termChanged(sleep = throttle) {
    if (authlevel >= roles.manager) {
      if (mode == "user") user = term;
      else if (mode == "project") project = term;
      else jobName = term;

      if (timeoutId != null) clearTimeout(timeoutId);

      timeoutId = setTimeout(() => {
        dispatch("update", {
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
        dispatch("update", {
          project,
          jobName
        });
      }, sleep);
    }
  }
</script>

<InputGroup>
  <select
    style="max-width: 175px;"
    class="form-select"
    bind:value={mode}
    on:change={modeChanged}
  >
    {#if authlevel >= roles.manager}
      <option value={"user"}>Search User</option>
    {/if}
    <option value={"project"}>Search Project</option>
    <option value={"jobName"}>Search Jobname</option>
  </select>
  <Input
    type="text"
    bind:value={term}
    on:change={() => termChanged()}
    on:keyup={(event) => termChanged(event.key == "Enter" ? 0 : throttle)}
    placeholder={`filter ${mode}...`}
  />
</InputGroup>

