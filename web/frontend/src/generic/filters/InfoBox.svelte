<!--
    @component Info pill displayed for active filters

    Properties:
    - `icon String`: Sveltestrap icon name
    - `modified Bool?`: Optional if filter is modified [Default: false]
 -->
 
 <script>
  import { Button, Icon } from "@sveltestrap/sveltestrap";

  export let icon;
  export let modified = false;
</script>

<Button class="mr-2 mb-1" outline color={modified ? "warning" : "primary"} on:click>
  <Icon name={icon} />
  <slot />
</Button>
