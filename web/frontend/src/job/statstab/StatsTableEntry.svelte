<!--
    @component Job-View subcomponent; Single Statistics entry component for statstable

    Properties:
    - `host String`: The hostname (== node)
    - `metric String`: The metric name
    - `scope String`: The selected scope
    - `data [Object]`: The jobs statsdata
 -->

<script>
  import { Icon } from "@sveltestrap/sveltestrap";

  export let host;
  export let metric;
  export let scope;
  export let data;

  let entrySorting = {
    id: { dir: "down", active: true },
    min: { dir: "up", active: false },
    avg: { dir: "up", active: false },
    max: { dir: "up", active: false },
  };

  function compareNumbers(a, b) {
    return a.id - b.id;
  }

  function sortByField(field) {
    let s = entrySorting[field];
    if (s.active) {
      s.dir = s.dir == "up" ? "down" : "up";
    } else {
      for (let field in entrySorting) entrySorting[field].active = false;
      s.active = true;
    }

    entrySorting = { ...entrySorting };
    stats = stats.sort((a, b) => {
      if (a == null || b == null) return -1;

      if (field === "id") {
        return s.dir != "up" ?
          a[field].localeCompare(b[field], undefined, {numeric: true, sensitivity: 'base'}) :
          b[field].localeCompare(a[field], undefined, {numeric: true, sensitivity: 'base'})
      } else {
        return s.dir != "up"
          ? a.data[field] - b.data[field]
          : b.data[field] - a.data[field];
      }
    });
  }

  $: stats = data
    ?.find((d) => d.name == metric && d.scope == scope)
    ?.stats.filter((s) => s.hostname == host && s.data != null)
    ?.sort(compareNumbers) || [];
</script>

{#if stats == null || stats.length == 0}
  <td colspan={scope == "node" ? 3 : 4}><i>No data</i></td>
{:else if stats.length == 1 && scope == "node"}
  <td>
    {stats[0].data.min}
  </td>
  <td>
    {stats[0].data.avg}
  </td>
  <td>
    {stats[0].data.max}
  </td>
{:else}
  <td colspan="4">
    <table style="width: 100%;">
      <tr>
        {#each ["id", "min", "avg", "max"] as field}
          <th on:click={() => sortByField(field)}>
            Sort
            <Icon
              name="caret-{entrySorting[field].dir}{entrySorting[field].active
                ? '-fill'
                : ''}"
            />
          </th>
        {/each}
      </tr>
      {#each stats as s, i}
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
