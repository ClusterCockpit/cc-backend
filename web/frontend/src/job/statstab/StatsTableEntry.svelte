<!--
    @component Job-View subcomponent; Single Statistics entry component for statstable

    Properties:
    - `data [Object]`: The jobs statsdata for host-metric-scope
    - `scope String`: The selected scope
 -->

<script>
  import { Icon } from "@sveltestrap/sveltestrap";

  /* Svelte 5 Props */
  let {
    data,
    scope,
  } = $props();

  /* State Init */
  let sortBy = $state("id");
  let sortDir = $state("down");

  /* Derived */
  const sortedData = $derived(updateData(data, sortBy, sortDir));

  /* Functions */
  function updateData(data, sortBy, sortDir) {
    data.sort((a, b) => {
      if (a == null || b == null) { 
        return -1;
      } else if (sortBy === "id") {
        return sortDir != "up"
          ? a[sortBy].localeCompare(b[sortBy], undefined, {numeric: true, sensitivity: 'base'})
          : b[sortBy].localeCompare(a[sortBy], undefined, {numeric: true, sensitivity: 'base'});
      } else {
        return sortDir != "up"
          ? a.data[sortBy] - b.data[sortBy]
          : b.data[sortBy] - a.data[sortBy];
      };
    });
    return [...data];
  };
</script>

{#if data == null || data.length == 0}
  <td colspan={scope == "node" ? 3 : 4}><i>No data</i></td>
{:else if data.length == 1 && scope == "node"}
  <td>
    {data[0].data.min}
  </td>
  <td>
    {data[0].data.avg}
  </td>
  <td>
    {data[0].data.max}
  </td>
{:else}
  <td colspan="4">
    <table style="width: 100%;">
      <tbody>
        <tr>
          {#each ["id", "min", "avg", "max"] as field}
            <th onclick={() => {
                sortBy = field; 
                sortDir = (sortDir == "up" ? "down" : "up");
              }}>
              Sort
              <Icon
                name="caret-{sortBy == field? sortDir: 'down'}{sortBy == field? '-fill': ''}"
              />
            </th>
          {/each}
        </tr>
        {#each sortedData as s, i}
          <tr>
            <th>{s.id ?? i}</th>
            <td>{s.data.min}</td>
            <td>{s.data.avg}</td>
            <td>{s.data.max}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </td>
{/if}
