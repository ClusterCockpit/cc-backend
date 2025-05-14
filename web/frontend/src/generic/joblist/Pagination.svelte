<!-- 
    @component Pagination selection component

    Properties:
    - page:         Number (changes from inside)
    - itemsPerPage: Number (changes from inside)
    - totalItems:   Number (only displayed)

    Events:
    - "update-paging": { page: Number, itemsPerPage: Number }
      - Dispatched once immediately and then each time page or itemsPerPage changes
 -->

<div class="cc-pagination" >
    <div class="cc-pagination-left">
        <label for="cc-pagination-select">{ itemText } per page:</label>
        <div class="cc-pagination-select-wrapper">
            <select on:blur|preventDefault={reset} bind:value={itemsPerPage} id="cc-pagination-select" class="cc-pagination-select">
                {#each pageSizes as size}
                    <option value="{size}">{size}</option>
                {/each}
            </select>
            <span class="focus"></span>
        </div>
        <span class="cc-pagination-text">
            { (page - 1) * itemsPerPage } - { Math.min((page - 1) * itemsPerPage + itemsPerPage, totalItems) } of { totalItems } { itemText }
        </span>
    </div>
    <div class="cc-pagination-right">
        {#if !backButtonDisabled}
        <button aria-label="page-reset"  class="reset nav" type="button"
            on:click|preventDefault={() => reset()}></button>
        <button aria-label="page-back" class="left nav" type="button"
            on:click|preventDefault={() => { page -= 1; }}></button>
        {/if}
        {#if !nextButtonDisabled}
        <button aria-label="page-up" class="right nav" type="button"
            on:click|preventDefault={() => { page += 1; }}></button>
        {/if}
    </div>
</div>

<script>
    import { createEventDispatcher } from "svelte";
    export let page = 1;
    export let itemsPerPage = 10;
    export let totalItems = 0;
    export let itemText = "items";
    export let pageSizes = [10,25,50];

    let backButtonDisabled, nextButtonDisabled;

    const dispatch = createEventDispatcher();

    $: {
        if (typeof page !== "number") {
            page = Number(page);
        }

        if (typeof itemsPerPage !== "number") {
            itemsPerPage = Number(itemsPerPage);
        }

        dispatch("update-paging", { itemsPerPage, page });
    }
    $: backButtonDisabled = (page === 1);
    $: nextButtonDisabled = (page >= (totalItems / itemsPerPage));

    function reset ( event ) {
        page = 1;
    }
</script>

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
