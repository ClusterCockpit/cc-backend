<!--
    @component Job-View subcomponent; Single Statistics entry component fpr statstable

    Properties:
    - `host String`: The hostname (== node)
    - `metric String`: The metric name
    - `scope String`: The selected scope
    - `jobMetrics [Object]`: The jobs metricdata
 -->

<script>
  import { Icon } from "@sveltestrap/sveltestrap";

  export let host;
  export let metric;
  export let scope;
  export let jobMetrics;

  function compareNumbers(a, b) {
    return a.id - b.id;
  }

  function sortByField(field) {
    let s = sorting[field];
    if (s.active) {
      s.dir = s.dir == "up" ? "down" : "up";
    } else {
      for (let field in sorting) sorting[field].active = false;
      s.active = true;
    }

    sorting = { ...sorting };
    series = series.sort((a, b) => {
      if (a == null || b == null) return -1;

      if (field === "id") {
        return s.dir != "up" ? a[field] - b[field] : b[field] - a[field];
      } else {
        return s.dir != "up"
          ? a.data[field] - b.data[field]
          : b.data[field] - a.data[field];
      }
    });
  }

  let sorting = {
    id: { dir: "down", active: true },
    min: { dir: "up", active: false },
    avg: { dir: "up", active: false },
    max: { dir: "up", active: false },
  };

  $: series = jobMetrics
    .find((jm) => jm.name == metric && jm.scope == scope)
    ?.stats.filter((s) => s.hostname == host && s.data != null)
    ?.sort(compareNumbers);
</script>

{#if series == null || series.length == 0}
  <td colspan={scope == "node" ? 3 : 4}><i>No data</i></td>
{:else if series.length == 1 && scope == "node"}
  <td>
    {series[0].data.min}
  </td>
  <td>
    {series[0].data.avg}
  </td>
  <td>
    {series[0].data.max}
  </td>
{:else}
  <td colspan="4">
    <table style="width: 100%;">
      <tr>
        {#each ["id", "min", "avg", "max"] as field}
          <th on:click={() => sortByField(field)}>
            Sort
            <Icon
              name="caret-{sorting[field].dir}{sorting[field].active
                ? '-fill'
                : ''}"
            />
          </th>
        {/each}
      </tr>
      {#each series as s, i}
        <tr>
          <th>{s.id ?? i}</th>
          <td>{s.data.min}</td>
          <td>{s.data.avg}</td>
          <td>{s.data.max}</td>
        </tr>
      {/each}
    </table>
  </td>
{/if}
