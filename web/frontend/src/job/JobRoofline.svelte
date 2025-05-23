<!--
    @component Job View Roofline component; Queries data for and renders roofline plot.

    Properties:
    - `job Object`: The GQL job object
    - `clusters Array`: The GQL clusters array
 -->

 <script>
    import { 
      queryStore,
      gql,
      getContextClient 
    } from "@urql/svelte";
    import {
      Card,
      Spinner
    } from "@sveltestrap/sveltestrap";
    import {
      transformDataForRoofline,
    } from "../generic/utils.js";
    import Roofline from "../generic/plots/Roofline.svelte";

    export let job;
    export let clusters;
  
    let roofWidth;

    const client = getContextClient();
    const roofQuery = gql`
        query ($dbid: ID!, $selectedMetrics: [String!]!, $selectedScopes: [MetricScope!]!, $selectedResolution: Int) {
            jobMetrics(id: $dbid, metrics: $selectedMetrics, scopes: $selectedScopes, resolution: $selectedResolution) {
            name
            scope
            metric {
                series {
                    data
                }
            }
            }
        }
    `;

    // Roofline: Always load roofMetrics with configured timestep (Resolution: 0)
    $: roofMetrics = queryStore({
        client: client,
        query: roofQuery,
        variables: { dbid: job.id, selectedMetrics: ["flops_any", "mem_bw"], selectedScopes: ["node"], selectedResolution: 0 },
    });

</script>

{#if $roofMetrics.error}
    <Card body color="danger">{$roofMetrics.error.message}</Card>
{:else if $roofMetrics?.data}
    <Card style="height: 400px;">
        <div bind:clientWidth={roofWidth}>
            <Roofline
                width={roofWidth}
                subCluster={clusters
                    .find((c) => c.name == job.cluster)
                    .subClusters.find((sc) => sc.name == job.subCluster)}
                    data={transformDataForRoofline(
                    $roofMetrics.data?.jobMetrics?.find(
                        (m) => m.name == "flops_any" && m.scope == "node",
                    )?.metric,
                    $roofMetrics.data?.jobMetrics?.find(
                        (m) => m.name == "mem_bw" && m.scope == "node",
                    )?.metric,
                    )}
                allowSizeChange
                renderTime
            />
        </div>
    </Card>
{:else}
    <Spinner secondary />
{/if}

