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
</script>

<style>
    a {
        margin-left: 0.5rem;
        line-height: 2;
    }
    span {
        font-size: 0.9rem;
    }
</style>

<a target={clickable ? "_blank" : null} href={clickable ? `/monitoring/jobs/?tag=${id}` : null}>
    {#if tag}
        {#if tag?.scope === "global"}
            <span style="background-color:#c85fc8;" class="badge text-dark">{tag.type}: {tag.name}</span>
        {:else if tag.scope === "admin"}
            <span style="background-color:#19e5e6;" class="badge text-dark">{tag.type}: {tag.name}</span>
        {:else}
            <span class="badge bg-warning text-dark">{tag.type}: {tag.name}</span>
        {/if}
    {:else}
        Loading...
    {/if}
</a>
