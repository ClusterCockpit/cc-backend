<script>
  import { getContext } from "svelte";
  import Refresher from "./joblist/Refresher.svelte";
  import Roofline from "./plots/Roofline.svelte";
  import Pie, { colors } from "./plots/Pie.svelte";
  import Histogram from "./plots/Histogram.svelte";
  import {
    Row,
    Col,
    Spinner,
    Card,
    CardHeader,
    CardTitle,
    CardBody,
    Table,
    Progress,
    Icon,
    Button,
    Modal,
    ModalHeader,
    ModalBody,
    ModalFooter,
    Accordion,
    AccordionItem,
  } from "sveltestrap";
  import { onMount, onDestroy } from "svelte";

  let screenSize = window.innerWidth;

  function updateScreenSize() {
    screenSize = window.innerWidth;
  }

  onMount(() => {
    window.addEventListener("resize", updateScreenSize);
  });

  onDestroy(() => {
    window.removeEventListener("resize", updateScreenSize);
  });

  import {
    init,
    convert2uplot,
    transformPerNodeDataForRoofline,
  } from "./utils.js";
  import { scaleNumbers } from "./units.js";
  import {
    queryStore,
    gql,
    getContextClient,
    mutationStore,
  } from "@urql/svelte";
  import PlotTable from "./PlotTable.svelte";
  import HistogramSelection from "./HistogramSelection.svelte";
  import ClusterMachine from "./partition/ClusterMachine.svelte";

  const { query: initq } = init();
  const ccconfig = getContext("cc-config");

  export let cluster;

  let plotWidths = [],
    colWidth1,
    colWidth2;
  let from = new Date(Date.now() - 5 * 60 * 1000),
    to = new Date(Date.now());
  const topOptions = [
    { key: "totalJobs", label: "Jobs" },
    { key: "totalNodes", label: "Nodes" },
    { key: "totalCores", label: "Cores" },
    { key: "totalAccs", label: "Accelerators" },
  ];

  let topProjectSelection =
    topOptions.find(
      (option) =>
        option.key ==
        ccconfig[`status_view_selectedTopProjectCategory:${cluster}`]
    ) ||
    topOptions.find(
      (option) => option.key == ccconfig.status_view_selectedTopProjectCategory
    );
  let topUserSelection =
    topOptions.find(
      (option) =>
        option.key == ccconfig[`status_view_selectedTopUserCategory:${cluster}`]
    ) ||
    topOptions.find(
      (option) => option.key == ccconfig.status_view_selectedTopUserCategory
    );

  let isHistogramSelectionOpen = false;
  $: metricsInHistograms = cluster
    ? ccconfig[`user_view_histogramMetrics:${cluster}`] || []
    : ccconfig.user_view_histogramMetrics || [];

  const client = getContextClient();
  $: mainQuery = queryStore({
    client: client,
    query: gql`
      query (
        $cluster: String!
        $filter: [JobFilter!]!
        $metrics: [String!]
        $from: Time!
        $to: Time!
        $metricsInHistograms: [String!]
      ) {
        nodeMetrics(
          cluster: $cluster
          metrics: $metrics
          from: $from
          to: $to
        ) {
          host
          subCluster
          metrics {
            name
            scope
            metric {
              timestep
              unit {
                base
                prefix
              }
              series {
                data
              }
            }
          }
        }

        stats: jobsStatistics(filter: $filter, metrics: $metricsInHistograms) {
          histDuration {
            count
            value
          }
          histNumNodes {
            count
            value
          }
          histNumCores {
            count
            value
          }
          histNumAccs {
            count
            value
          }
          histMetrics {
            metric
            unit
            data {
              min
              max
              count
              bin
            }
          }
        }

        allocatedNodes(cluster: $cluster) {
          name
          count
        }
      }
    `,
    variables: {
      cluster: cluster,
      metrics: ["flops_any", "mem_bw"],
      from: from.toISOString(),
      to: to.toISOString(),
      filter: [{ state: ["running"] }, { cluster: { eq: cluster } }],
      metricsInHistograms: metricsInHistograms,
    },
  });

  const paging = { itemsPerPage: 10, page: 1 }; // Top 10
  $: topUserQuery = queryStore({
    client: client,
    query: gql`
      query (
        $filter: [JobFilter!]!
        $paging: PageRequest!
        $sortBy: SortByAggregate!
      ) {
        topUser: jobsStatistics(
          filter: $filter
          page: $paging
          sortBy: $sortBy
          groupBy: USER
        ) {
          id
          totalJobs
          totalNodes
          totalCores
          totalAccs
        }
      }
    `,
    variables: {
      filter: [{ state: ["running"] }, { cluster: { eq: cluster } }],
      paging,
      sortBy: topUserSelection.key.toUpperCase(),
    },
  });

  $: topProjectQuery = queryStore({
    client: client,
    query: gql`
      query (
        $filter: [JobFilter!]!
        $paging: PageRequest!
        $sortBy: SortByAggregate!
      ) {
        topProjects: jobsStatistics(
          filter: $filter
          page: $paging
          sortBy: $sortBy
          groupBy: PROJECT
        ) {
          id
          totalJobs
          totalNodes
          totalCores
          totalAccs
        }
      }
    `,
    variables: {
      filter: [{ state: ["running"] }, { cluster: { eq: cluster } }],
      paging,
      sortBy: topProjectSelection.key.toUpperCase(),
    },
  });

  const sumUp = (data, subcluster, metric) =>
    data.reduce(
      (sum, node) =>
        node.subCluster == subcluster
          ? sum +
            (node.metrics
              .find((m) => m.name == metric)
              ?.metric.series.reduce(
                (sum, series) => sum + series.data[series.data.length - 1],
                0
              ) || 0)
          : sum,
      0
    );

  let allocatedNodes = {},
    flopRate = {},
    flopRateUnitPrefix = {},
    flopRateUnitBase = {},
    memBwRate = {},
    memBwRateUnitPrefix = {},
    memBwRateUnitBase = {};
  $: if ($initq.data && $mainQuery.data) {
    let subClusters = $initq.data.clusters.find(
      (c) => c.name == cluster
    ).subClusters;
    for (let subCluster of subClusters) {
      allocatedNodes[subCluster.name] =
        $mainQuery.data.allocatedNodes.find(
          ({ name }) => name == subCluster.name
        )?.count || 0;
      flopRate[subCluster.name] =
        Math.floor(
          sumUp($mainQuery.data.nodeMetrics, subCluster.name, "flops_any") * 100
        ) / 100;
      flopRateUnitPrefix[subCluster.name] = subCluster.flopRateSimd.unit.prefix;
      flopRateUnitBase[subCluster.name] = subCluster.flopRateSimd.unit.base;
      memBwRate[subCluster.name] =
        Math.floor(
          sumUp($mainQuery.data.nodeMetrics, subCluster.name, "mem_bw") * 100
        ) / 100;
      memBwRateUnitPrefix[subCluster.name] =
        subCluster.memoryBandwidth.unit.prefix;
      memBwRateUnitBase[subCluster.name] = subCluster.memoryBandwidth.unit.base;
    }
  }

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

  function updateTopUserConfiguration(select) {
    if (ccconfig[`status_view_selectedTopUserCategory:${cluster}`] != select) {
      updateConfigurationMutation({
        name: `status_view_selectedTopUserCategory:${cluster}`,
        value: JSON.stringify(select),
      }).subscribe((res) => {
        if (res.fetching === false && !res.error) {
          // console.log(`status_view_selectedTopUserCategory:${cluster}` + ' -> Updated!')
        } else if (res.fetching === false && res.error) {
          throw res.error;
        }
      });
    } else {
      // console.log('No Mutation Required: Top User')
    }
  }

  function updateTopProjectConfiguration(select) {
    if (
      ccconfig[`status_view_selectedTopProjectCategory:${cluster}`] != select
    ) {
      updateConfigurationMutation({
        name: `status_view_selectedTopProjectCategory:${cluster}`,
        value: JSON.stringify(select),
      }).subscribe((res) => {
        if (res.fetching === false && !res.error) {
          // console.log(`status_view_selectedTopProjectCategory:${cluster}` + ' -> Updated!')
        } else if (res.fetching === false && res.error) {
          throw res.error;
        }
      });
    } else {
      // console.log('No Mutation Required: Top Project')
    }
  }

  $: updateTopUserConfiguration(topUserSelection.key);
  $: updateTopProjectConfiguration(topProjectSelection.key);
</script>

<!-- Loading indicator & Refresh -->

<Row>
  <Col xs="auto" style="align-self: flex-end;">
    <h4 class="mb-0">Current utilization of cluster "{cluster}"</h4>
  </Col>
  <Col xs="auto" style="margin-left: 0.25rem;">
    {#if $initq.fetching || $mainQuery.fetching}
      <Spinner />
    {:else if $initq.error}
      <Card body color="danger">{$initq.error.message}</Card>
    {:else}
      <!-- ... -->
    {/if}
  </Col>
  <Col xs="auto" style="margin-left: auto;">
    <Button
      outline
      color="secondary"
      on:click={() => (isHistogramSelectionOpen = true)}
    >
      <Icon name="bar-chart-line" /> Select Histograms
    </Button>
  </Col>
  <Col xs="auto" style="margin-left: 0.25rem;">
    <Refresher
      initially={120}
      on:reload={() => {
        from = new Date(Date.now() - 5 * 60 * 1000);
        to = new Date(Date.now());
      }}
    />
  </Col>
</Row>
{#if $mainQuery.error}
  <Row>
    <Col>
      <Card body color="danger">{$mainQuery.error.message}</Card>
    </Col>
  </Row>
{/if}

<hr />

<ClusterMachine />  