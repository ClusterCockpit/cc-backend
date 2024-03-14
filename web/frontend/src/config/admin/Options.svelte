<script>
  import { onMount } from "svelte";
  import { Card, CardBody, CardTitle } from "@sveltestrap/sveltestrap";

  let scrambled;

  onMount(() => {
    scrambled = window.localStorage.getItem("cc-scramble-names") != null;
  });

  function handleScramble() {
    if (!scrambled) {
      scrambled = true;
      window.localStorage.setItem("cc-scramble-names", "true");
    } else {
      scrambled = false;
      window.localStorage.removeItem("cc-scramble-names");
    }
  }
</script>

<Card class="h-100">
  <CardBody>
    <CardTitle class="mb-3">Scramble Names / Presentation Mode</CardTitle>
    <input
      type="checkbox"
      id="scramble-names-checkbox"
      style="margin-right: 1em;"
      on:click={handleScramble}
      bind:checked={scrambled}
    />
    Active?
  </CardBody>
</Card>
