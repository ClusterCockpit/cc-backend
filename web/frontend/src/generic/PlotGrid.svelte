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

    export let itemsPerRow
    export let items
</script>

<Row cols={{ xs: 1, sm: 2, md: 3, lg: itemsPerRow}}>
  {#each items as item}
    <Col class="px-1">
      <slot {item}/>
    </Col>
  {/each}
</Row>

