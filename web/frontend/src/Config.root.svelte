<!--
    @component Main Config Option Component, Wrapper for admin and user sub-components

    Properties:
    - `ìsAdmin Bool!`: Is currently logged in user admin authority
    - `isApi Bool!`: Is currently logged in user api authority
    - `username String!`: Empty string if auth. is disabled, otherwise the username as string
 -->

<script>
  import { Card, CardHeader, CardTitle } from "@sveltestrap/sveltestrap";
  import UserSettings from "./config/UserSettings.svelte";
  import AdminSettings from "./config/AdminSettings.svelte";

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
