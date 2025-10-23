<!--
    @component Support settings wrapper
    Properties: None
 -->

<script>
  import { getContext } from "svelte";
  import SupportOptions from "./support/SupportOptions.svelte";

  const ccconfig = getContext("cc-config");
</script>

<SupportOptions config={ccconfig}/>
