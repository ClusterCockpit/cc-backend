<!--
  @component Admin option select card

    Properties:
  - `clusterNames [String]`: The available clusternames
-->

<script>
  import { getContext, onMount } from "svelte";
  import { Row, Col, Card, CardBody, CardTitle, Button, Icon } from "@sveltestrap/sveltestrap";

  /* Svelte 5 Props */
  let {
    clusterNames,
  } = $props();

  /*Const Init */
  const resampleConfig = getContext("resampling");
  
  /* State Init */
  let scrambled = $state(false);
  
  /* on Mount */
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
        onclick={() => handleScramble()}
        bind:checked={scrambled}
      />
      Active?
    </CardBody>
  </Card>
</Col>

{#if clusterNames?.length > 0}
  <Col> 
    <Card class="h-100">
      <CardBody>
        <CardTitle class="mb-3">Public Dashboard Links</CardTitle>
        <Row>
        {#each clusterNames as cn}
          <Col>
            <Button color="info" class="mb-2 mb-xl-0" href={`/monitoring/dashboard/${cn}`} target="_blank">
              <Icon name="clipboard-pulse" class="mr-2"/>
              {cn.charAt(0).toUpperCase() + cn.slice(1)} Public Dashboard
            </Button>
          </Col>
        {/each}
        </Row>
      </CardBody>
    </Card>
  </Col>
{/if}

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
