<!-- 
  @component Pagination selection component

  Properties:
  - `page Number?`: Current page [Default: 1]
  - `itemsPerPage Number?`: Current items displayed per page [Default: 10]
  - `totalItems Number?`: Total count of items [Default: 0]
  - `itemText String?`: Name of paged items, e.g. "Jobs" [Default: "items"]
  - `pageSizes [Number!]?`: Options available for page sizes [Default: [10, 25, 50]]
  - `updatePaging Func`: The callback function to apply current paging selection
-->

<script>
  /* Svelte 5 Props */
  let {
    page = 1,
    itemsPerPage = 10,
    totalItems = 0,
    itemText = "items",
    pageSizes = [10, 25, 50],
    updatePaging
  } = $props();

  /* Derived */
  const backButtonDisabled = $derived((page === 1));
  const nextButtonDisabled = $derived((page >= (totalItems / itemsPerPage)));

  /* Functions */
  function pageUp ( event ) {
    event.preventDefault();
    page += 1;
    updatePaging({ itemsPerPage, page });
  }
  
  function pageBack ( event ) {
    event.preventDefault();
    page -= 1;
    updatePaging({ itemsPerPage, page });
  }

  function pageReset ( event ) {
    event.preventDefault();
    page = 1;
    updatePaging({ itemsPerPage, page });
  }

  function updateItems ( event ) {
    event.preventDefault();
    updatePaging({ itemsPerPage, page });
  }
</script>

<div class="cc-pagination" >
  <div class="cc-pagination-left">
    <label for="cc-pagination-select">{ itemText } per page:</label>
    <div class="cc-pagination-select-wrapper">
      <select onblur={(e) => pageReset(e)} onchange={(e) => updateItems(e)} bind:value={itemsPerPage} id="cc-pagination-select" class="cc-pagination-select">
        {#each pageSizes as size}
          <option value="{size}">{size}</option>
        {/each}
      </select>
      <span class="focus"></span>
    </div>
    <span class="cc-pagination-text">
      { ((page - 1) * itemsPerPage) + 1 } - { Math.min((page - 1) * itemsPerPage + itemsPerPage, totalItems) } of { totalItems } { itemText }
    </span>
  </div>
  <div class="cc-pagination-right">
    {#if !backButtonDisabled}
      <button aria-label="page-reset"  class="reset nav" type="button"
        onclick={(e) => pageReset(e)}></button>
      <button aria-label="page-back" class="left nav" type="button"
        onclick={(e) => pageBack(e)}></button>
    {/if}
    {#if !nextButtonDisabled}
      <button aria-label="page-up" class="right nav" type="button"
        onclick={(e) => pageUp(e)}></button>
    {/if}
  </div>
</div>

<style>
  *, *::before, *::after {
    box-sizing: border-box;
  }

  div {
    display: flex;
    align-items: center;
    vertical-align: baseline;
    box-sizing: border-box;
  }

  label, select, button {
    margin: 0;
    padding: 0;
    vertical-align: baseline;
    color: #525252;
  }

  button {
    position: relative;
    border: none;
    border-left: 1px solid #e0e0e0;
    height: 3rem;
    width: 3rem;
    background: 0 0;
    transition: all 70ms;
  }

  button:hover {
    background-color: #dde1e6;
  }

  button:focus {
    top: -1px;
    left: -1px;
    right: -1px;
    bottom: -1px;
    border: 1px solid blue;
    border-radius: inherit;
  }

  .nav::after {
    content: "";
    width: 0.9em;
    height: 0.8em;
    background-color: #777;
    z-index: 1;
    position: absolute;
    top: 50%;
    left: 50%;
  }

  .nav:disabled {
    background-color: #fff;
    cursor: no-drop;
  }

  .reset::after {
    clip-path: polygon(100% 0%, 75% 50%, 100% 100%, 25% 100%, 0% 50%, 25% 0%);
    margin-top: -0.3em;
    margin-left: -0.5em;
  }

  .right::after {
    clip-path: polygon(100% 50%, 50% 0, 50% 100%);
    margin-top: -0.3em;
    margin-left: -0.5em;
  }

  .left::after {
    clip-path: polygon(50% 0, 0 50%, 50% 100%);
    margin-top: -0.3em;
    margin-left: -0.3em;
  }

  .cc-pagination-select-wrapper::after {
    content: "";
    width: 0.8em;
    height: 0.5em;
    background-color: #777;
    clip-path: polygon(100% 0%, 0 0%, 50% 100%);
    justify-self: end;
  }

  .cc-pagination {
    width: 100%;
    justify-content: space-between;
    border-top: 1px solid #e0e0e0;
  }

  .cc-pagination-text {
    color: #525252;
    margin-left: 1rem;
  }

  .cc-pagination-text {
    color: #525252;
    margin-right: 1rem;
  }

  .cc-pagination-left {
    padding: 0 1rem;
    height: 3rem;
  }

  .cc-pagination-select-wrapper {
    display: grid;
    grid-template-areas: "select";
    align-items: center;
    position: relative;
    padding: 0 0.5em;
    min-width: 3em;
    max-width: 6em;
    border-right: 1px solid #e0e0e0;
    cursor: pointer;
    transition: all 70ms;
  }

  .cc-pagination-select-wrapper:hover {
    background-color: #dde1e6;
  }

  select,
  .cc-pagination-select-wrapper::after {
    grid-area: select;
  }

  .cc-pagination-select {
    height: 3rem;
    appearance: none;
    background-color: transparent;
    padding: 0 1em 0 0;
    margin: 0;
    border: none;
    width: 100%;
    font-family: inherit;
    font-size: inherit;
    cursor: inherit;
    line-height: inherit;
    z-index: 1;
    outline: none;
  }

  select:focus + .focus {
    position: absolute;
    top: -1px;
    left: -1px;
    right: -1px;
    bottom: -1px;
    border: 1px solid blue;
    border-radius: inherit;
  }

  .cc-pagination-right {
    height: 3rem;
  }
</style>
