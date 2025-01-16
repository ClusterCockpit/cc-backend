import { expiringCacheExchange } from "./cache-exchange.js";
import {
    Client,
    setContextClient,
    fetchExchange,
} from "@urql/svelte";
import { setContext, getContext, hasContext, onDestroy, tick } from "svelte";
import { readable } from "svelte/store";

/*
 * Call this function only at component initialization time!
 *
 * It does several things:
 * - Initialize the GraphQL client
 * - Creates a readable store 'initialization' which indicates when the values below can be used.
 * - Adds 'tags' to the context (list of all tags)
 * - Adds 'clusters' to the context (object with cluster names as keys)
 * - Adds 'globalMetrics' to the context (list of globally available metric infos)
 * - Adds 'getMetricConfig' to the context, a function that takes a cluster, subCluster and metric name and returns the MetricConfig (or undefined)
 * - Adds 'getHardwareTopology' to the context, a function that takes a cluster nad subCluster and returns the subCluster topology (or undefined)
 */
export function init(extraInitQuery = "") {
    const jwt = hasContext("jwt")
        ? getContext("jwt")
        : getContext("cc-config")["jwt"];

    const client = new Client({
        url: `${window.location.origin}/query`,
        fetchOptions:
            jwt != null ? { headers: { Authorization: `Bearer ${jwt}` } } : {},
        exchanges: [
            expiringCacheExchange({
                ttl: 5 * 60 * 1000,
                maxSize: 150,
            }),
            fetchExchange,
        ],
    });

    setContextClient(client);

    const query = client
        .query(
            `query {
        clusters {
            name
            partitions
            subClusters {
                name
                nodes
                numberOfNodes
                processorType
                socketsPerNode
                coresPerSocket
                threadsPerCore
                flopRateScalar { unit { base, prefix }, value }
                flopRateSimd { unit { base, prefix }, value }
                memoryBandwidth { unit { base, prefix }, value }
                topology {
                    node
                    socket
                    core
                    accelerators { id }
                }
                metricConfig {
                    name
                    unit { base, prefix }
                    scope
                    aggregation
                    timestep
                    peak
                    normal
                    caution
                    alert
                    lowerIsBetter
                }
                footprint 
            }
        }
        tags { id, name, type, scope }
        globalMetrics {
            name
            scope
            footprint
            unit { base, prefix }
            availability { cluster, subClusters }
        }
        ${extraInitQuery}
    }`
        )
        .toPromise();

    let state = { fetching: true, error: null, data: null };
    let subscribers = [];
    const subscribe = (callback) => {
        callback(state);
        subscribers.push(callback);
        return () => {
            subscribers = subscribers.filter((cb) => cb != callback);
        };
    };

    const tags = []
    const clusters = []
    const globalMetrics = []

    setContext("tags", tags);
    setContext("clusters", clusters);
    setContext("globalMetrics", globalMetrics);
    setContext("getMetricConfig", (cluster, subCluster, metric) => {
        // Load objects if input is string
        if (typeof cluster !== "object")
            cluster = clusters.find((c) => c.name == cluster);
        if (typeof subCluster !== "object")
            subCluster = cluster.subClusters.find((sc) => sc.name == subCluster);

        return subCluster.metricConfig.find((m) => m.name == metric);
    });
    setContext("getHardwareTopology", (cluster, subCluster) => {
        // Load objects if input is string
        if (typeof cluster !== "object")
            cluster = clusters.find((c) => c.name == cluster);
        if (typeof subCluster !== "object")
            subCluster = cluster.subClusters.find((sc) => sc.name == subCluster);

        return subCluster?.topology;
    });
    setContext("on-init", (callback) =>
        state.fetching ? subscribers.push(callback) : callback(state)
    );
    setContext(
        "initialized",
        readable(false, (set) => subscribers.push(() => set(true)))
    );

    query.then(({ error, data }) => {
        state.fetching = false;
        if (error != null) {
            console.error(error);
            state.error = error;
            tick().then(() => subscribers.forEach((cb) => cb(state)));
            return;
        }

        for (let tag of data.tags) tags.push(tag);
        for (let cluster of data.clusters) clusters.push(cluster);
        for (let gm of data.globalMetrics) globalMetrics.push(gm);

        // Unified Sort
        globalMetrics.sort((a, b) => a.name.localeCompare(b.name))

        state.data = data;
        tick().then(() => subscribers.forEach((cb) => cb(state)));
    });

    return {
        query: { subscribe },
        tags,
        clusters,
        globalMetrics
    };
}

// Use https://developer.mozilla.org/en-US/docs/Web/API/structuredClone instead?
export function deepCopy(x) {
    return JSON.parse(JSON.stringify(x));
}

function fuzzyMatch(term, string) {
    return string.toLowerCase().includes(term);
}

// Use in filter() function to return only unique values
export function distinct(value, index, array) {
    return array.indexOf(value) === index;
}

// Load Local Bool and Handle Scrambling of input string
export const scrambleNames = window.localStorage.getItem("cc-scramble-names");
export const scramble = function (str) {
  if (str === "-") return str;
  else
    return [...str]
      .reduce((x, c, i) => x * 7 + c.charCodeAt(0) * i * 21, 5)
      .toString(32)
      .substr(0, 6);
};

export function fuzzySearchTags(term, tags) {
    if (!tags) return [];

    let results = [];
    let termparts = term
        .split(":")
        .map((s) => s.trim())
        .filter((s) => s.length > 0);

    if (termparts.length == 0) {
        results = tags.slice();
    } else if (termparts.length == 1) {
        for (let tag of tags)
            if (
                fuzzyMatch(termparts[0], tag.type) ||
                fuzzyMatch(termparts[0], tag.name)
            )
                results.push(tag);
    } else if (termparts.length == 2) {
        for (let tag of tags)
            if (
                fuzzyMatch(termparts[0], tag.type) &&
                fuzzyMatch(termparts[1], tag.name)
            )
                results.push(tag);
    }

    return results.sort((a, b) => {
        if (a.type < b.type) return -1;
        if (a.type > b.type) return 1;
        if (a.name < b.name) return -1;
        if (a.name > b.name) return 1;
        return 0;
    });
}

export function groupByScope(jobMetrics) {
    let metrics = new Map();
    for (let metric of jobMetrics) {
        if (metrics.has(metric.name)) metrics.get(metric.name).push(metric);
        else metrics.set(metric.name, [metric]);
    }

    return [...metrics.values()].sort((a, b) =>
        a[0].name.localeCompare(b[0].name)
    );
}

const scopeGranularity = {
    node: 10,
    socket: 5,
    memorydomain: 4,
    core: 3,
    hwthread: 2,
    accelerator: 1
};

export function maxScope(scopes) {
    console.assert(
        scopes.length > 0 && scopes.every((x) => scopeGranularity[x] != null)
    );
    let sm = scopes[0],
        gran = scopeGranularity[scopes[0]];
    for (let scope of scopes) {
        let otherGran = scopeGranularity[scope];
        if (otherGran > gran) {
            sm = scope;
            gran = otherGran;
        }
    }
    return sm;
}

export function minScope(scopes) {
    console.assert(
        scopes.length > 0 && scopes.every((x) => scopeGranularity[x] != null)
    );
    let sm = scopes[0],
        gran = scopeGranularity[scopes[0]];
    for (let scope of scopes) {
        let otherGran = scopeGranularity[scope];
        if (otherGran < gran) {
            sm = scope;
            gran = otherGran;
        }
    }
    return sm;
}

export function stickyHeader(datatableHeaderSelector, updatePading) {
    const header = document.querySelector("header > nav.navbar");
    if (!header) return;

    let ticking = false,
        datatableHeader = null;
    const onscroll = (event) => {
        if (ticking) return;

        ticking = true;
        window.requestAnimationFrame(() => {
            ticking = false;
            if (!datatableHeader)
                datatableHeader = document.querySelector(datatableHeaderSelector);

            const top = datatableHeader.getBoundingClientRect().top;
            updatePading(
                top < header.clientHeight ? header.clientHeight - top + 10 : 10
            );
        });
    };

    document.addEventListener("scroll", onscroll);
    onDestroy(() => document.removeEventListener("scroll", onscroll));
}

export function checkMetricDisabled(m, c, s) { // [m]etric, [c]luster, [s]ubcluster
    const metrics = getContext("globalMetrics");
    const result = metrics?.find((gm) => gm.name === m)?.availability?.find((av) => av.cluster === c)?.subClusters?.includes(s)
    return !result
}

export function getStatsItems(presetStats = []) {
    // console.time('stats')
    const globalMetrics = getContext("globalMetrics")
    const result = globalMetrics.map((gm) => {
        if (gm?.footprint) {
            const mc = getMetricConfigDeep(gm.name, null, null)
            if (mc) {
                const presetEntry = presetStats.find((s) => s?.field === (gm.name + '_' + gm.footprint))
                if (presetEntry) {
                    return {
                        field: gm.name + '_' + gm.footprint,
                        text: gm.name + ' (' + gm.footprint + ')',
                        metric: gm.name,
                        from: presetEntry.from,
                        to: presetEntry.to,
                        peak: mc.peak,
                        enabled: true
                    }
                } else {
                    return {
                        field: gm.name + '_' + gm.footprint,
                        text: gm.name + ' (' + gm.footprint + ')',
                        metric: gm.name,
                        from: 0,
                        to: mc.peak,
                        peak: mc.peak,
                        enabled: false
                    }
                }
            }
        }
        return null
    }).filter((r) => r != null)
    // console.timeEnd('stats')
    return [...result];
};

export function getSortItems() {
    //console.time('sort')
    const globalMetrics = getContext("globalMetrics")
    const result = globalMetrics.map((gm) => {
        if (gm?.footprint) {
            return { 
                field: gm.name + '_' + gm.footprint,
                type: 'foot',
                text: gm.name + ' (' + gm.footprint + ')',
                order: 'DESC'
            }
        }
        return null
    }).filter((r) => r != null)
    //console.timeEnd('sort')
    return [...result];
};

function getMetricConfigDeep(metric, cluster, subCluster) {
    const clusters = getContext("clusters");
    if (cluster != null) {
        const c = clusters.find((c) => c.name == cluster);
        if (subCluster != null) {
            const sc = c.subClusters.find((sc) => sc.name == subCluster);
            return sc.metricConfig.find((mc) => mc.name == metric)
        } else {
            let result;
            for (let sc of c.subClusters) {
                const mc = sc.metricConfig.find((mc) => mc.name == metric)
                if (result && mc) { // update result; If lowerIsBetter: Peak is still maximum value, no special case required
                    result.alert = (mc.alert > result.alert) ? mc.alert : result.alert
                    result.caution = (mc.caution > result.caution) ? mc.caution : result.caution
                    result.normal = (mc.normal > result.normal) ? mc.normal : result.normal
                    result.peak = (mc.peak > result.peak) ? mc.peak : result.peak
                } else if (mc) {
                    // start new result
                    result = {...mc};
                }
            }
            return result
        }
    } else {
        let result;
        for (let c of clusters) {
            for (let sc of c.subClusters) {
                const mc = sc.metricConfig.find((mc) => mc.name == metric)
                if (result && mc) { // update result; If lowerIsBetter: Peak is still maximum value, no special case required
                    result.alert = (mc.alert > result.alert) ? mc.alert : result.alert
                    result.caution = (mc.caution > result.caution) ? mc.caution : result.caution
                    result.normal = (mc.normal > result.normal) ? mc.normal : result.normal
                    result.peak = (mc.peak > result.peak) ? mc.peak : result.peak
                } else if (mc) {
                    // Start new result
                    result = {...mc};
                }
            }
        }
        return result
    }
}

export function convert2uplot(canvasData, minutesToHours = false) {
    // Prep: Uplot Data Structure
    let uplotData = [[],[]] // [X, Y1, Y2, ...]
    // Iterate if exists
    if (canvasData) {
        canvasData.forEach( cd => {
            if (Object.keys(cd).length == 4) { // MetricHisto Datafromat
                uplotData[0].push(cd?.max ? cd.max : 0)
                uplotData[1].push(cd.count)
            } else { // Default
                if (minutesToHours) {
                    let hours = cd.value / 60
                    console.log("x minutes to y hours", cd.value, hours)
                    uplotData[0].push(hours)
                } else {
                    uplotData[0].push(cd.value)
                }
                uplotData[1].push(cd.count)
             }
        })
    }
    return uplotData
}

export function binsFromFootprint(weights, scope, values, numBins) {
    let min = 0, max = 0 //, median = 0
    if (values.length != 0) {
        // Extreme, wrong peak vlaues: Filter here or backend?
        // median = median(values)

        for (let x of values) {
            min = Math.min(min, x)
            max = Math.max(max, x)
        }
        max += 1 // So that we have an exclusive range.
    }

    if (numBins == null || numBins < 3)
        numBins = 3

    let scopeWeights
    switch (scope) {
        case 'core':
            scopeWeights = weights.coreHours
            break
        case 'accelerator':
            scopeWeights = weights.accHours
            break
        default: // every other scope: use 'node'
            scopeWeights = weights.nodeHours
    }

    const rawBins = new Array(numBins).fill(0)
    for (let i = 0; i < values.length; i++)
        rawBins[Math.floor(((values[i] - min) / (max - min)) * numBins)] += scopeWeights ? scopeWeights[i] : 1

    const bins = rawBins.map((count, idx) => ({ 
        value: Math.floor(min + ((idx + 1) / numBins) * (max - min)),
        count: count 
    }))

    return {
        bins: bins
    }
}

export function transformDataForRoofline(flopsAny, memBw) { // Uses Metric Objects: {series:[{},{},...], timestep:60, name:$NAME}
    /* c will contain values from 0 to 1 representing the time */
    let data = null
    const x = [], y = [], c = []

    if (flopsAny && memBw) {
        const nodes = flopsAny.series.length
        const timesteps = flopsAny.series[0].data.length

        for (let i = 0; i < nodes; i++) {
            const flopsData = flopsAny.series[i].data
            const memBwData = memBw.series[i].data
            for (let j = 0; j < timesteps; j++) {
                const f = flopsData[j], m = memBwData[j]
                const intensity = f / m
                if (Number.isNaN(intensity) || !Number.isFinite(intensity))
                    continue

                x.push(intensity)
                y.push(f)
                c.push(j / timesteps)
            }
        }
    } else {
        console.warn("transformData: metrics for 'mem_bw' and/or 'flops_any' missing!")
    }
    if (x.length > 0 && y.length > 0 && c.length > 0) {
        data = [null, [x, y], c] // for dataformat see roofline.svelte
    }
    return data
}

//  Return something to be plotted. The argument shall be the result of the
// `nodeMetrics` GraphQL query.
// Hardcoded metric names required for correct render
export function transformPerNodeDataForRoofline(nodes) {
    let data = null
    const x = [], y = []
    for (let node of nodes) {
        let flopsAny = node.metrics.find(m => m.name == 'flops_any' && m.scope == 'node')?.metric
        let memBw    = node.metrics.find(m => m.name == 'mem_bw'    && m.scope == 'node')?.metric
        if (!flopsAny || !memBw) {
            console.warn("transformPerNodeData: metrics for 'mem_bw' and/or 'flops_any' missing!")
            continue
        }

        let flopsData = flopsAny.series[0].data, memBwData = memBw.series[0].data
        const f = flopsData[flopsData.length - 1], m = memBwData[flopsData.length - 1]
        const intensity = f / m
        if (Number.isNaN(intensity) || !Number.isFinite(intensity))
            continue

        x.push(intensity)
        y.push(f)
    }
    if (x.length > 0 && y.length > 0) {
        data = [null, [x, y], []] // for dataformat see roofline.svelte
    }
    return data
}

export async function fetchJwt(username) {
    const raw = await fetch(`/frontend/jwt/?username=${username}`);

    if (!raw.ok) {
        const message = `An error has occured: ${response.status}`;
        throw new Error(message);
    }

    const res = await raw.text();
    return res;
}

// https://stackoverflow.com/questions/45309447/calculating-median-javascript
// function median(numbers) {
//     const sorted = Array.from(numbers).sort((a, b) => a - b);
//     const middle = Math.floor(sorted.length / 2);
  
//     if (sorted.length % 2 === 0) {
//         return (sorted[middle - 1] + sorted[middle]) / 2;
//     }
  
//     return sorted[middle];
// }
