<!--
    @component Main cluster status view component; renders current system-usage information

    Properties:
    - `cluster String`: The cluster to show status information for
 -->
 
 <script>
  import { getContext } from "svelte";
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
  } from "@sveltestrap/sveltestrap";
  import {
    queryStore,
    gql,
    getContextClient,
    mutationStore,
  } from "@urql/svelte";
  import {
    init,
    convert2uplot,
    transformPerNodeDataForRoofline,
  } from "./generic/utils.js";
  import { scaleNumbers } from "./generic/units.js";
  import PlotTable from "./generic/PlotTable.svelte";
  import Roofline from "./generic/plots/Roofline.svelte";
  import Pie, { colors } from "./generic/plots/Pie.svelte";
  import Histogram from "./generic/plots/Histogram.svelte";
  import Refresher from "./generic/helper/Refresher.svelte";
  import HistogramSelection from "./generic/select/HistogramSelection.svelte";

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
        ccconfig[`status_view_selectedTopProjectCategory:${cluster}`],
    ) ||
    topOptions.find(
      (option) => option.key == ccconfig.status_view_selectedTopProjectCategory,
    );
  let topUserSelection =
    topOptions.find(
      (option) =>
        option.key ==
        ccconfig[`status_view_selectedTopUserCategory:${cluster}`],
    ) ||
    topOptions.find(
      (option) => option.key == ccconfig.status_view_selectedTopUserCategory,
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
      metrics: ["flops_any", "mem_bw"], // Fixed names for roofline and status bars
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
                0,
              ) || 0)
          : sum,
      0,
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
      (c) => c.name == cluster,
    ).subClusters;
    for (let subCluster of subClusters) {
      allocatedNodes[subCluster.name] =
        $mainQuery.data.allocatedNodes.find(
          ({ name }) => name == subCluster.name,
        )?.count || 0;
      flopRate[subCluster.name] =
        Math.floor(
          sumUp($mainQuery.data.nodeMetrics, subCluster.name, "flops_any") *
            100,
        ) / 100;
      flopRateUnitPrefix[subCluster.name] = subCluster.flopRateSimd.unit.prefix;
      flopRateUnitBase[subCluster.name] = subCluster.flopRateSimd.unit.base;
      memBwRate[subCluster.name] =
        Math.floor(
          sumUp($mainQuery.data.nodeMetrics, subCluster.name, "mem_bw") * 100,
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

<Row cols={{ lg: 3, md: 3, sm: 1 }}>
  <Col style="">
    <h4 class="mb-0">Current utilization of cluster "{cluster}"</h4>
  </Col>
  <Col class="mt-2 mt-md-0 text-md-end">
    <Button
      outline
      color="secondary"
      on:click={() => (isHistogramSelectionOpen = true)}
    >
      <Icon name="bar-chart-line" /> Select Histograms
    </Button>
  </Col>
  <Col class="mt-2 mt-md-0">
    <Refresher
      initially={120}
      on:refresh={() => {
        from = new Date(Date.now() - 5 * 60 * 1000);
        to = new Date(Date.now());
      }}
    />
  </Col>
</Row>
<Row cols={1} class="text-center mt-3">
  <Col>
    {#if $initq.fetching || $mainQuery.fetching}
      <Spinner />
    {:else if $initq.error}
      <Card body color="danger">{$initq.error.message}</Card>
    {:else}
      <!-- ... -->
    {/if}
  </Col>
</Row>
{#if $mainQuery.error}
  <Row cols={1}>
    <Col>
      <Card body color="danger">{$mainQuery.error.message}</Card>
    </Col>
  </Row>
{/if}

<hr />

<!-- Gauges & Roofline per Subcluster-->

{#if $initq.data && $mainQuery.data}
  {#each $initq.data.clusters.find((c) => c.name == cluster).subClusters as subCluster, i}
    <Row cols={{ lg: 2, md: 1 , sm: 1}} class="mb-3 justify-content-center">
      <Col class="px-3">
        <Card class="h-auto mt-1">
          <CardHeader>
            <CardTitle class="mb-0">SubCluster "{subCluster.name}"</CardTitle>
          </CardHeader>
          <CardBody>
            <Table borderless>
              <tr class="py-2">
                <th scope="col">Allocated Nodes</th>
                <td style="min-width: 100px;"
                  ><div class="col">
                    <Progress
                      value={allocatedNodes[subCluster.name]}
                      max={subCluster.numberOfNodes}
                    />
                  </div></td
                >
                <td
                  >{allocatedNodes[subCluster.name]} / {subCluster.numberOfNodes}
                  Nodes</td
                >
              </tr>
              <tr class="py-2">
                <th scope="col"
                  >Flop Rate (Any) <Icon
                    name="info-circle"
                    class="p-1"
                    style="cursor: help;"
                    title="Flops[Any] = (Flops[Double] x 2) + Flops[Single]"
                  /></th
                >
                <td style="min-width: 100px;"
                  ><div class="col">
                    <Progress
                      value={flopRate[subCluster.name]}
                      max={subCluster.flopRateSimd.value *
                        subCluster.numberOfNodes}
                    />
                  </div></td
                >
                <td>
                  {scaleNumbers(
                    flopRate[subCluster.name],
                    subCluster.flopRateSimd.value * subCluster.numberOfNodes,
                    flopRateUnitPrefix[subCluster.name],
                  )}{flopRateUnitBase[subCluster.name]} [Max]
                </td>
              </tr>
              <tr class="py-2">
                <th scope="col">MemBw Rate</th>
                <td style="min-width: 100px;"
                  ><div class="col">
                    <Progress
                      value={memBwRate[subCluster.name]}
                      max={subCluster.memoryBandwidth.value *
                        subCluster.numberOfNodes}
                    />
                  </div></td
                >
                <td>
                  {scaleNumbers(
                    memBwRate[subCluster.name],
                    subCluster.memoryBandwidth.value * subCluster.numberOfNodes,
                    memBwRateUnitPrefix[subCluster.name],
                  )}{memBwRateUnitBase[subCluster.name]} [Max]
                </td>
              </tr>
            </Table>
          </CardBody>
        </Card>
      </Col>
      <Col class="px-3 mt-2 mt-lg-0">
        <div bind:clientWidth={plotWidths[i]}>
          {#key $mainQuery.data.nodeMetrics}
            <Roofline
              allowSizeChange={true}
              width={plotWidths[i] - 10}
              height={300}
              subCluster={subCluster}
              data={transformPerNodeDataForRoofline(
                $mainQuery.data.nodeMetrics.filter(
                  (data) => data.subCluster == subCluster.name,
                ),
              )}
            />
          {/key}
        </div>
      </Col>
    </Row>
  {/each}

  <hr />

  <!-- Usage Stats as Histograms -->

  <Row cols={{ lg: 4, md: 2, sm: 1 }}>
    <Col class="p-2">
      <div bind:clientWidth={colWidth1}>
        <h4 class="text-center">
          Top Users on {cluster.charAt(0).toUpperCase() + cluster.slice(1)}
        </h4>
        {#key $topUserQuery.data}
          {#if $topUserQuery.fetching}
            <Spinner />
          {:else if $topUserQuery.error}
            <Card body color="danger">{$topUserQuery.error.message}</Card>
          {:else}
            <Pie
              size={colWidth1}
              sliceLabel={topUserSelection.label}
              quantities={$topUserQuery.data.topUser.map(
                (tu) => tu[topUserSelection.key],
              )}
              entities={$topUserQuery.data.topUser.map((tu) => tu.id)}
            />
          {/if}
        {/key}
      </div>
    </Col>
    <Col class="px-4 py-2">
      {#key $topUserQuery.data}
        {#if $topUserQuery.fetching}
          <Spinner />
        {:else if $topUserQuery.error}
          <Card body color="danger">{$topUserQuery.error.message}</Card>
        {:else}
          <Table>
            <tr class="mb-2">
              <th>Legend</th>
              <th>User Name</th>
              <th
                >Number of
                <select class="p-0" bind:value={topUserSelection}>
                  {#each topOptions as option}
                    <option value={option}>
                      {option.label}
                    </option>
                  {/each}
                </select>
              </th>
            </tr>
            {#each $topUserQuery.data.topUser as tu, i}
              <tr>
                <td><Icon name="circle-fill" style="color: {colors[i]};" /></td>
                <th scope="col"
                  ><a
                    href="/monitoring/user/{tu.id}?cluster={cluster}&state=running"
                    >{tu.id}</a
                  ></th
                >
                <td>{tu[topUserSelection.key]}</td>
              </tr>
            {/each}
          </Table>
        {/if}
      {/key}
    </Col>
    <Col class="p-2">
      <h4 class="text-center">
        Top Projects on {cluster.charAt(0).toUpperCase() + cluster.slice(1)}
      </h4>
      {#key $topProjectQuery.data}
        {#if $topProjectQuery.fetching}
          <Spinner />
        {:else if $topProjectQuery.error}
          <Card body color="danger">{$topProjectQuery.error.message}</Card>
        {:else}
          <Pie
            size={colWidth1}
            sliceLabel={topProjectSelection.label}
            quantities={$topProjectQuery.data.topProjects.map(
              (tp) => tp[topProjectSelection.key],
            )}
            entities={$topProjectQuery.data.topProjects.map((tp) => tp.id)}
          />
        {/if}
      {/key}
    </Col>
    <Col class="px-4 py-2">
      {#key $topProjectQuery.data}
        {#if $topProjectQuery.fetching}
          <Spinner />
        {:else if $topProjectQuery.error}
          <Card body color="danger">{$topProjectQuery.error.message}</Card>
        {:else}
          <Table>
            <tr class="mb-2">
              <th>Legend</th>
              <th>Project Code</th>
              <th
                >Number of
                <select class="p-0" bind:value={topProjectSelection}>
                  {#each topOptions as option}
                    <option value={option}>
                      {option.label}
                    </option>
                  {/each}
                </select>
              </th>
            </tr>
            {#each $topProjectQuery.data.topProjects as tp, i}
              <tr>
                <td><Icon name="circle-fill" style="color: {colors[i]};" /></td>
                <th scope="col"
                  ><a
                    href="/monitoring/jobs/?cluster={cluster}&state=running&project={tp.id}&projectMatch=eq"
                    >{tp.id}</a
                  ></th
                >
                <td>{tp[topProjectSelection.key]}</td>
              </tr>
            {/each}
          </Table>
        {/if}
      {/key}
    </Col>
  </Row>
  <hr class="my-2" />
  <Row cols={{ lg: 2, md: 1 }}>
    <Col class="p-2">
      <div bind:clientWidth={colWidth2}>
        {#key $mainQuery.data.stats}
          <Histogram
            data={convert2uplot($mainQuery.data.stats[0].histDuration)}
            width={colWidth2 - 25}
            title="Duration Distribution"
            xlabel="Current Runtimes"
            xunit="Hours"
            ylabel="Number of Jobs"
            yunit="Jobs"
          />
        {/key}
      </div>
    </Col>
    <Col class="p-2">
      {#key $mainQuery.data.stats}
        <Histogram
          data={convert2uplot($mainQuery.data.stats[0].histNumNodes)}
          width={colWidth2 - 25}
          title="Number of Nodes Distribution"
          xlabel="Allocated Nodes"
          xunit="Nodes"
          ylabel="Number of Jobs"
          yunit="Jobs"
        />
      {/key}
    </Col>
  </Row>
  <Row cols={{ lg: 2, md: 1 }}>
    <Col class="p-2">
      <div bind:clientWidth={colWidth2}>
        {#key $mainQuery.data.stats}
          <Histogram
            data={convert2uplot($mainQuery.data.stats[0].histNumCores)}
            width={colWidth2 - 25}
            title="Number of Cores Distribution"
            xlabel="Allocated Cores"
            xunit="Cores"
            ylabel="Number of Jobs"
            yunit="Jobs"
          />
        {/key}
      </div>
    </Col>
    <Col class="p-2">
      {#key $mainQuery.data.stats}
        <Histogram
          data={convert2uplot($mainQuery.data.stats[0].histNumAccs)}
          width={colWidth2 - 25}
          title="Number of Accelerators Distribution"
          xlabel="Allocated Accs"
          xunit="Accs"
          ylabel="Number of Jobs"
          yunit="Jobs"
        />
      {/key}
    </Col>
  </Row>
  <hr class="my-2" />
  {#if metricsInHistograms}
    <Row cols={1}>
      <Col>
        {#key $mainQuery.data.stats[0].histMetrics}
          <PlotTable
            let:item
            let:width
            renderFor="user"
            items={$mainQuery.data.stats[0].histMetrics}
            itemsPerRow={2}
          >
            <Histogram
              data={convert2uplot(item.data)}
              usesBins={true}
              {width}
              height={250}
              title="Distribution of '{item.metric}' averages"
              xlabel={`${item.metric} bin maximum ${item?.unit ? `[${item.unit}]` : ``}`}
              xunit={item.unit}
              ylabel="Number of Jobs"
              yunit="Jobs"
            />
          </PlotTable>
        {/key}
      </Col>
    </Row>
  {/if}
{/if}

<HistogramSelection
  bind:cluster
  bind:metricsInHistograms
  bind:isOpen={isHistogramSelectionOpen}
/>
