<!-- 
    @component Single tag pill component

    Properties:
    - id: ID! (if the tag-id is known but not the tag type/name, this can be used)
    - tag: { id: ID!, type: String, name: String }
    - clickable: Boolean (default is true)
 -->

<script>
    import { getContext } from 'svelte'
    const allTags = getContext('tags'),
          initialized = getContext('initialized')

    export let id = null
    export let tag = null
    export let clickable = true

    if (tag != null && id == null)
        id = tag.id

    $: {
        if ($initialized && tag == null)
            tag = allTags.find(tag => tag.id == id)
    }

    function getScopeColor(scope) {
        switch (scope) {
        case "admin":
            return "#19e5e6";
        case "global":
            return "#c85fc8";
        default:
            return "#ffc107";
        }
    }
</script>

<style>
    a {
        margin-right: 0.5rem;
    }
    span {
        font-size: 0.9rem;
    }
</style>

<a target={clickable ? "_blank" : null} href={clickable ? `/monitoring/jobs/?tag=${id}` : null}>
    {#if tag}
        <span style="background-color:{getScopeColor(tag?.scope)};" class="my-1 badge text-dark">{tag.type}: {tag.name}</span>
    {:else}
        Loading...
    {/if}
</a>
