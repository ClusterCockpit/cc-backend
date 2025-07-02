<!-- 
  @component Single tag pill component

  Properties:
  - `id ID!`: (if the tag-id is known but not the tag type/name, this can be used)
  - `tag Object`: The tag Object
  - `clickable Bool`: If tag should be click reactive [Default: true]
-->

<script>
  import { getContext } from 'svelte'
  
  /* Svelte 5 Props */
  let {
    id = null,
    tag = null,
    clickable = true
  } = $props();

  /* Derived */
  const allTags = $derived(getContext('tags'));
  const initialized = $derived(getContext('initialized'));

  /* Effects */
  $effect(() => {
    if (tag != null && id == null)
      id = tag.id
  });

  $effect(() => {
    if ($initialized && tag == null)
      tag = allTags.find(tag => tag.id == id)
  });

  /* Function*/
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

<a target={clickable ? "_blank" : null} href={clickable ? `/monitoring/jobs/?tag=${id}` : null}>
  {#if tag}
    <span style="background-color:{getScopeColor(tag?.scope)};" class="my-1 badge text-dark">{tag.type}: {tag.name}</span>
  {:else}
    Loading...
  {/if}
</a>

<style>
  a {
    margin-right: 0.5rem;
  }
  span {
    font-size: 0.9rem;
  }
</style>
