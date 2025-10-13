<!--
  @component Analysis-View subcomponent; allows selection for normalized histograms and scatterplots

  Properties:
  - `availableMetrics [String]`: Available metrics in selected cluster
  - `presetMetricsInHistograms [String]`: The latest selected metrics to display as histogram
  - `presetMetricsInScatterplots [[String, String]]`: The latest selected metrics to display as scatterplot
  - `applyHistograms Func`: The callback function to apply current histogramMetrics selection
  - `applyScatter Func`: The callback function to apply current scatterMetrics selection
-->

<script>
  import {
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
    InputGroup,
    Button,
    ListGroup,
    ListGroupItem,
    Icon,
  } from "@sveltestrap/sveltestrap";
  import { gql, getContextClient, mutationStore } from "@urql/svelte";

  /* Svelte 5 Props */
  let {
    availableMetrics,
    presetMetricsInHistograms,
    presetMetricsInScatterplots,
    applyHistograms,
    applyScatter
  } = $props();

  /* Const Init */
  const client = getContextClient();
  const updateConfigurationMutation = ({ name, value }) => {
    return mutationStore({
      client: client,
      query: gql`
        mutation ($name: String!, $value: String!) {
          updateConfiguration(name: $name, value: $value)
        }
      `,
      variables: { name, value },
    });
  };

  /* State Init */
  let isHistogramConfigOpen = $state(false);
  let isScatterPlotConfigOpen = $state(false);
  let metricsInHistograms = $state(presetMetricsInHistograms);
  let metricsInScatterplots = $state(presetMetricsInScatterplots);
  let selectedMetric1 = $state(null);
  let selectedMetric2 = $state(null);

  /* Functions */
  function updateConfiguration(data) {
    updateConfigurationMutation({
      name: data.name,
      value: JSON.stringify(data.value),
    }).subscribe((res) => {
      if (res.fetching === false && res.error) {
        throw res.error;
      }
    });
  }
</script>

<Button outline onclick={() => (isHistogramConfigOpen = true)}>
  <Icon name="" />
  Select Plots for Histograms
</Button>

<Button outline onclick={() => (isScatterPlotConfigOpen = true)}>
  <Icon name="" />
  Select Plots in Scatter Plots
</Button>

<Modal
  isOpen={isHistogramConfigOpen}
  toggle={() => (isHistogramConfigOpen = !isHistogramConfigOpen)}
>
  <ModalHeader>Select metrics presented in histograms</ModalHeader>
  <ModalBody>
    <ListGroup>
      {#each availableMetrics as metric (metric)}
        <ListGroupItem>
          <input
            type="checkbox"
            bind:group={metricsInHistograms}
            value={metric}
            onchange={() => {
              updateConfiguration({
                name: "analysisView_histogramMetrics",
                value: metricsInHistograms,
              });
              applyHistograms(metricsInHistograms);
            }}
          />

          {metric}
        </ListGroupItem>
      {/each}
    </ListGroup>
  </ModalBody>
  <ModalFooter>
    <Button color="primary" onclick={() => (isHistogramConfigOpen = false)}>
      Close
    </Button>
  </ModalFooter>
</Modal>

<Modal
  isOpen={isScatterPlotConfigOpen}
  toggle={() => (isScatterPlotConfigOpen = !isScatterPlotConfigOpen)}
>
  <ModalHeader>Select metric pairs presented in scatter plots</ModalHeader>
  <ModalBody>
    <ListGroup>
      {#each metricsInScatterplots as pair}
        <ListGroupItem>
          <b>{pair[0]}</b> / <b>{pair[1]}</b>

          <Button
            style="float: right;"
            outline
            color="danger"
            onclick={() => {
              metricsInScatterplots = metricsInScatterplots.filter(
                (p) => pair != p,
              );
              updateConfiguration({
                name: "analysisView_scatterPlotMetrics",
                value: metricsInScatterplots,
              });
              applyScatter(metricsInScatterplots);
            }}
          >
            <Icon name="x" />
          </Button>
        </ListGroupItem>
      {/each}
    </ListGroup>

    <br />

    <InputGroup>
      <select bind:value={selectedMetric1} class="form-group form-select">
        <option value={null}>Choose Metric for X Axis</option>
        {#each availableMetrics as metric}
          <option value={metric}>{metric}</option>
        {/each}
      </select>
      <select bind:value={selectedMetric2} class="form-group form-select">
        <option value={null}>Choose Metric for Y Axis</option>
        {#each availableMetrics as metric}
          <option value={metric}>{metric}</option>
        {/each}
      </select>
      <Button
        outline
        disabled={selectedMetric1 == null || selectedMetric2 == null}
        onclick={() => {
          metricsInScatterplots = [
            ...metricsInScatterplots,
            [selectedMetric1, selectedMetric2],
          ];
          selectedMetric1 = null;
          selectedMetric2 = null;
          updateConfiguration({
            name: "analysisView_scatterPlotMetrics",
            value: metricsInScatterplots,
          });
          applyScatter(metricsInScatterplots);
        }}
      >
        Add Plot
      </Button>
    </InputGroup>
  </ModalBody>
  <ModalFooter>
    <Button color="primary" onclick={() => (isScatterPlotConfigOpen = false)}>
      Close
    </Button>
  </ModalFooter>
</Modal>
