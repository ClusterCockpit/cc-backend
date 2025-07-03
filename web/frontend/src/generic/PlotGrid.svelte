<!-- 
  @component Organized display of svelte 5 snippets as bootstrap (sveltestrap) grid

  Properties:
  - `items [Any]`: Array of information required for gridContent SV5 snippet
  - `itemsPerRow Number`: Elements to render per row
  - `gridContent Func`: Svelte 5 Snippet from Upstream; Defines what and how to render $item data
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

