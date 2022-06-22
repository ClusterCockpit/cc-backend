<!-- 
    @component

    Properties:
    - itemsPerRow: Number
    - items:       [Any]
 -->

<script>
    export let itemsPerRow
    export let items
    export let padding = 10

    let tableWidth = 0
    const PLACEHOLDER = { magic: 'object' }

    function tile(items, itemsPerRow) {
        const rows = []
        for (let ri = 0; ri < items.length; ri += itemsPerRow) {
            const row = []
            for (let ci = 0; ci < itemsPerRow; ci += 1) {
                if (ri + ci < items.length)
                    row.push(items[ri + ci])
                else
                    row.push(PLACEHOLDER)
            }

            rows.push(row)
        }

        return rows
    }

    $: rows = tile(items, itemsPerRow)
    $: plotWidth = (tableWidth / itemsPerRow) - (padding * itemsPerRow)
</script>

<table bind:clientWidth={tableWidth} style="width: 100%; table-layout: fixed;">
    {#each rows as row}
        <tr>
            {#each row as item (item)}
                <td>
                    {#if item != PLACEHOLDER && plotWidth > 0}
                        <slot item={item} width={plotWidth}></slot>
                    {/if}
                </td>
            {/each}
        </tr>
    {/each}
</table>
