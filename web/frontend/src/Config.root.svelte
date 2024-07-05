<script>
  // import { init } from "./utils.js";
  import { Card, CardHeader, CardTitle } from "@sveltestrap/sveltestrap";

  import UserSettings from "./config/UserSettings.svelte";
  import AdminSettings from "./config/AdminSettings.svelte";

  // const { query: initq } = init();

  export let isAdmin;
  export let isApi;
  export let username;
</script>

{#if isAdmin == true}
  <Card style="margin-bottom: 1.5em;">
    <CardHeader>
      <CardTitle class="mb-1">Admin Options</CardTitle>
    </CardHeader>
    <AdminSettings />
  </Card>
{/if}

<Card>
  <CardHeader>
    <CardTitle class="mb-1">User Options</CardTitle>
  </CardHeader>
  <UserSettings {username} {isApi}/>
</Card>
