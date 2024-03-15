<script>
  import { Button, Icon } from "@sveltestrap/sveltestrap";

  export let icon;
  export let modified = false;
</script>

<Button outline color={modified ? "warning" : "primary"} on:click>
  <Icon name={icon} />
  <slot />
</Button>
