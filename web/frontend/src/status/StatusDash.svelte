<!--
  @component Main cluster status view component; renders current system-usage information

  Properties:
  - `cluster String`: The cluster to show status information for
-->

 <script>
  import {
    Row,
    Col,
    Card,
    CardHeader,
    CardTitle,
    CardBody,
    Table,
    Progress,
    Icon,
  } from "@sveltestrap/sveltestrap";
  import {
    queryStore,
    gql,
    getContextClient,
  } from "@urql/svelte";
  import {
    init,
    transformPerNodeDataForRoofline,

  } from "../generic/utils.js";
  import { scaleNumbers } from "../generic/units.js";
  import Roofline from "../generic/plots/Roofline.svelte";

  /* Svelte 5 Props */
  let {
    cluster
  } = $props();

  /* Const Init */
  const { query: initq } = init();
  const client = getContextClient();

  /* State Init */
  let from = $state(new Date(Date.now() - 5 * 60 * 1000));
  let to = $state(new Date(Date.now()));
  let plotWidths = $state([]);
  // Bar Gauges
  let allocatedNodes = $state({});
  let flopRate = $state({});
  let flopRateUnitPrefix = $state({});
  let flopRateUnitBase = $state({});
  let memBwRate = $state({});
  let memBwRateUnitPrefix = $state({});
  let memBwRateUnitBase = $state({});

  /* Derived */
  // Note: nodeMetrics are requested on configured $timestep resolution
  const statusQuery = $derived(queryStore({
    client: client,
    query: gql`
      query (
        $cluster: String!
        $metrics: [String!]
        $from: Time!
        $to: Time!
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
      // filter: [{ state: ["running"] }, { cluster: { eq: cluster } }],
    },
  }));

  /* Effects */
  $effect(() => {
    if ($initq.data && $statusQuery.data) {
      let subClusters = $initq.data.clusters.find(
        (c) => c.name == cluster,
      ).subClusters;
      for (let subCluster of subClusters) {
        allocatedNodes[subCluster.name] =
          $statusQuery.data.allocatedNodes.find(
            ({ name }) => name == subCluster.name,
          )?.count || 0;
        flopRate[subCluster.name] =
          Math.floor(
            sumUp($statusQuery.data.nodeMetrics, subCluster.name, "flops_any") *
              100,
          ) / 100;
        flopRateUnitPrefix[subCluster.name] = subCluster.flopRateSimd.unit.prefix;
        flopRateUnitBase[subCluster.name] = subCluster.flopRateSimd.unit.base;
        memBwRate[subCluster.name] =
          Math.floor(
            sumUp($statusQuery.data.nodeMetrics, subCluster.name, "mem_bw") * 100,
          ) / 100;
        memBwRateUnitPrefix[subCluster.name] =
          subCluster.memoryBandwidth.unit.prefix;
        memBwRateUnitBase[subCluster.name] = subCluster.memoryBandwidth.unit.base;
      }
    }
  });

  /* Const Functions */
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

</script>

<!-- Gauges & Roofline per Subcluster-->
{#if $initq.data && $statusQuery.data}
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
          {#key $statusQuery.data.nodeMetrics}
            <Roofline
              allowSizeChange
              width={plotWidths[i] - 10}
              height={300}
              subCluster={subCluster}
              data={transformPerNodeDataForRoofline(
                $statusQuery.data.nodeMetrics.filter(
                  (data) => data.subCluster == subCluster.name,
                ),
              )}
            />
          {/key}
        </div>
      </Col>
    </Row>
  {/each}
{/if}
