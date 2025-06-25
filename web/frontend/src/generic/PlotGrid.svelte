<!-- 
    @component Organized display of plots as bootstrap (sveltestrap) grid

    Properties:
    - `itemsPerRow Number`: Elements to render per row
    - `items [Any]`: List of plot components to render
 -->

 <script>
    import {
        Row,
        Col,
    } from "@sveltestrap/sveltestrap";

    /* Svelte 5 Props */
    let {
      items,
      itemsPerRow,
      gridContent
    } = $props();
</script>

<Row cols={{ xs: 1, sm: 2, md: 3, lg: itemsPerRow}}>
  {#each items as item}
    <Col class="px-1">
      <!-- Note: Ignore '@' Error in IDE -->
      {@render gridContent(item)}
    </Col>
  {/each}
</Row>

