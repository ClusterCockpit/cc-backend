<!--
    @component Info pill displayed for active filters

    Properties:
    - `icon String`: Sveltestrap icon name
    - `modified Bool?`: Optional if filter is modified [Default: false]
    - `onclick Fn()`: Opens Modal on click
    - `children Fn()?`: Internal prop, Svelte 5 version of <slot/>
 -->
 
 <script>
  import { Button, Icon } from "@sveltestrap/sveltestrap";
  /* Svelte 5 Props */
  let { icon, modified, onclick, children } = $props();
</script>

<Button class="mr-2 mb-1" outline color={modified ? "warning" : "primary"} {onclick}>
  <Icon name={icon} />
  {#if children}
    <!-- Note: Ignore '@' Error in IDE -->
    {@render children()}
  {:else}
    <span>No content found</span>
  {/if}
</Button>
