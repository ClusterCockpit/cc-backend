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

    $: rows = tile(items, itemsPerRow)
    $: plotWidth = (tableWidth / itemsPerRow) - (padding * itemsPerRow)
</script>

<table bind:clientWidth={tableWidth} style="width: 100%; table-layout: fixed;">
    {#each rows as row}
        <tr>
            {#each row as item (item)}
                <td>
                    {#if !isPlaceholder(item) && plotWidth > 0}
                        <slot item={item} width={plotWidth}></slot>
                    {/if}
                </td>
            {/each}
        </tr>
    {/each}
</table>
