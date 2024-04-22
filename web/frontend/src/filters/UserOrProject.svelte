<script>
  import { InputGroup, Input } from "@sveltestrap/sveltestrap";
  import { createEventDispatcher } from "svelte";

  const dispatch = createEventDispatcher();

  export let user = "";
  export let project = "";
  export let authlevel;
  export let roles;
  let mode = "user",
    term = "";
  const throttle = 500;

  function modeChanged() {
    if (mode == "user") {
      project = term;
      term = user;
    } else {
      user = term;
      term = project;
    }
    termChanged(0);
  }

  let timeoutId = null;
  // Compatibility: Handle "user role" and "no role" identically
  function termChanged(sleep = throttle) {
    if (authlevel >= roles.manager) {
      if (mode == "user") user = term;
      else project = term;

      if (timeoutId != null) clearTimeout(timeoutId);

      timeoutId = setTimeout(() => {
        dispatch("update", {
          user,
          project,
        });
      }, sleep);
    } else {
      project = term;
      if (timeoutId != null) clearTimeout(timeoutId);

      timeoutId = setTimeout(() => {
        dispatch("update", {
          project,
        });
      }, sleep);
    }
  }
</script>

{#if authlevel >= roles.manager}
  <InputGroup>
    <select
      style="max-width: 175px;"
      class="form-select"
      bind:value={mode}
      on:change={modeChanged}
    >
      <option value={"user"}>Search User</option>
      <option value={"project"}>Search Project</option>
    </select>
    <Input
      type="text"
      bind:value={term}
      on:change={() => termChanged()}
      on:keyup={(event) => termChanged(event.key == "Enter" ? 0 : throttle)}
      placeholder={mode == "user" ? "filter username..." : "filter project..."}
    />
  </InputGroup>
{:else}
  <!-- Compatibility: Handle "user role" and "no role" identically-->
  <InputGroup>
    <Input
      type="text"
      bind:value={term}
      on:change={() => termChanged()}
      on:keyup={(event) => termChanged(event.key == "Enter" ? 0 : throttle)}
      placeholder="filter project..."
    />
  </InputGroup>
{/if}
