<!--
    @component Admin option select card
 -->

<script>
  import { getContext, onMount } from "svelte";
  import { Col, Card, CardBody, CardTitle } from "@sveltestrap/sveltestrap";

  let scrambled;

  const resampleConfig = getContext("resampling");

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

<Col>
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
</Col>

{#if resampleConfig}
  <Col> 
    <Card class="h-100">
      <CardBody>
        <CardTitle class="mb-3">Metric Plot Resampling Info</CardTitle>
        <p>Triggered at {resampleConfig.trigger} datapoints.</p>
        <p>Configured resolutions: {resampleConfig.resolutions}</p>
      </CardBody>
    </Card>
  </Col>
{/if}
