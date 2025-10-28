<!--
  @component Support settings wrapper
-->

<script>
  import { getContext } from "svelte";
  import SupportOptions from "./support/SupportOptions.svelte";

  /* Const Init */
  const ccconfig = getContext("cc-config");
</script>

<SupportOptions config={ccconfig}/>
