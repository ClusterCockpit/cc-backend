<script>
    import { init, checkMetricDisabled } from "./utils.js";
    import Refresher from "./joblist/Refresher.svelte";
    import {
        Row,
        Col,
        Input,
        InputGroup,
        InputGroupText,
        Icon,
        Spinner,
        Card,
        CardBody,
        CardFooter,
        CardHeader,
        CardSubtitle,
        CardText,
        CardTitle,
        Accordion,
        AccordionItem,
    } from "sveltestrap";
    import { queryStore, gql, getContextClient } from "@urql/svelte";
    import TimeSelection from "./filters/TimeSelection.svelte";
    import PlotTable from "./PlotTable.svelte";
    import MetricPlot from "./plots/MetricPlot.svelte";
    import { getContext } from "svelte";
    import VerticalTab from "./partition/VerticalTab.svelte";


    export let cluster;
    export let from = null;
    export let to = null;

    const { query: initq } = init();

    if (from == null || to == null) {
        to = new Date(Date.now());
        from = new Date(to.getTime());
        from.setMinutes(from.getMinutes() - 30);
    }

    const clusters = getContext("clusters");
    console.log(clusters);
    const ccconfig = getContext("cc-config");
    const metricConfig = getContext("metrics");

    let plotHeight = 300;
    let hostnameFilter = "";
    let selectedMetric = ccconfig.system_view_selectedMetric;

    const client = getContextClient();
    $: nodesQuery = queryStore({
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
                                statistics {
                                    min
                                    avg
                                    max
                                }
                                data
                            }
                        }
                    }
                }
            }
        `,
        variables: {
            cluster: cluster,
            metrics: [selectedMetric],
            from: from.toISOString(),
            to: to.toISOString(),
        },
    });

    let metricUnits = {};
    $: if ($nodesQuery.data) {
        let thisCluster = clusters.find((c) => c.name == cluster);
        if (thisCluster) {
            for (let metric of thisCluster.metricConfig) {
                if (metric.unit.prefix || metric.unit.base) {
                    metricUnits[metric.name] =
                        "(" +
                        (metric.unit.prefix ? metric.unit.prefix : "") +
                        (metric.unit.base ? metric.unit.base : "") +
                        ")";
                } else {
                    // If no unit defined: Omit Unit Display
                    metricUnits[metric.name] = "";
                }
            }
        }
    }

    let notifications = [
        {
            type: "success",
            message: "This is a success notification!",
        },
        {
            type: "error",
            message: "An error occurred.",
        },
        {
            type: "info",
            message: "Just a friendly reminder.",
        },
    ];
</script>

<Row>
    {#if $initq.error}
        <Card body color="danger">{$initq.error.message}</Card>
    {:else if $initq.fetching}
        <Spinner />
    {:else}
        <Col>
            <Refresher
                on:reload={() => {
                    const diff = Date.now() - to;
                    from = new Date(from.getTime() + diff);
                    to = new Date(to.getTime() + diff);
                }}
            />
        </Col>
        <Col>
            <TimeSelection bind:from bind:to />
        </Col>
        <Col>
            <InputGroup>
                <InputGroupText><Icon name="graph-up" /></InputGroupText>
                <InputGroupText>Metric</InputGroupText>
                <select class="form-select" bind:value={selectedMetric}>
                    {#each clusters.find((c) => c.name == cluster).metricConfig as metric}
                        <option value={metric.name}
                            >{metric.name} {metricUnits[metric.name]}</option
                        >
                    {/each}
                </select>
            </InputGroup>
        </Col>
        <Col>
            <InputGroup>
                <InputGroupText><Icon name="hdd" /></InputGroupText>
                <InputGroupText>Find Node</InputGroupText>
                <Input
                    placeholder="hostname..."
                    type="text"
                    bind:value={hostnameFilter}
                />
            </InputGroup>
        </Col>
    {/if}
</Row>
<br />

<Card>
    <CardHeader>
        <CardTitle>Storage Partition Graph</CardTitle>
    </CardHeader>
    <CardBody class="h5">
        <CardSubtitle>Realtime storage partition comes right here</CardSubtitle>
        <CardText></CardText>
    </CardBody>
</Card>
<br />
<!-- notification -->
<!-- <Accordion open={0}>
    {#each notifications as notification, i}
        <AccordionItem key={i}>
            <div
                class="d-flex justify-content-between align-items-center"
                role="button"
                tabindex={i}
            >
                <span class="me-2">
                    <i class={`bi bi-circle-fill text-${notification.type}`}
                    ></i>
                    {notification.type.toUpperCase()}
                </span>
                <i class="bi bi-chevron-down" />
            </div>
            <div class="collapse show">
                {notification.message}
            </div>
        </AccordionItem>
    {/each}
</Accordion> -->
<!-- <br /> -->

<Card class="mb-1">
    <CardHeader>
        <CardTitle>Partition Configuration</CardTitle>
    </CardHeader>
    <CardBody class="h5">
        <CardSubtitle>Create and manage LVM partitions</CardSubtitle>
        <CardText></CardText>
        <VerticalTab />
    </CardBody>
</Card>
<br/>
