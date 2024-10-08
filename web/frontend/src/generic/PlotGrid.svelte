<!-- 
    @component Organized display of plots as bootstrap (sveltestrap) grid

    Properties:
    - `itemsPerRow Number`: Elements to render per row
    - `items [Any]`: List of plot components to render
    - `renderFor String`:   If 'job', filter disabled metrics
 -->

 <script>
    import {
        Row,
        Col,
    } from "@sveltestrap/sveltestrap";

    export let itemsPerRow
    export let items
    export let renderFor

    let rows = []
    const isPlaceholder = x => x._is_placeholder === true

    function tile(items, itemsPerRow) {
        const rows = []
        for (let ri = 0; ri < items.length; ri += itemsPerRow) {
            const row = []
            for (let ci = 0; ci < itemsPerRow; ci += 1) {
                if (ri + ci < items.length)
                    row.push(items[ri + ci])
                else
                    row.push({ _is_placeholder: true, ri, ci })
            }
            rows.push(row)
        }
        return rows
    }

    $: if (renderFor === 'job') {
        rows = tile(items.filter(item => item.disabled === false), itemsPerRow)
    } else {
        rows = tile(items, itemsPerRow)
    }

</script>

{#each rows as row}
  <Row cols={{ xs: 1, sm: 1, md: 2, lg: itemsPerRow}}>
    {#each row as item (item)}
      <Col class="px-1">
        {#if !isPlaceholder(item)}
          <slot item={item}/>
        {/if}
      </Col>
    {/each}
  </Row>
{/each}

