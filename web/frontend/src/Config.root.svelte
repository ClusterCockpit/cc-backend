<!--
    @component Main Config Option Component, Wrapper for admin and user sub-components

    Properties:
    - `ìsAdmin Bool!`: Is currently logged in user admin authority
    - `isSupport Bool!`: Is currently logged in user support authority
    - `isApi Bool!`: Is currently logged in user api authority
    - `username String!`: Empty string if auth. is disabled, otherwise the username as string
 -->

<script>
  import { Card, CardHeader, CardTitle } from "@sveltestrap/sveltestrap";
  import UserSettings from "./config/UserSettings.svelte";
  import SupportSettings from "./config/SupportSettings.svelte";
  import AdminSettings from "./config/AdminSettings.svelte";

  export let isAdmin;
  export let isSupport;
  export let isApi;
  export let username;
  export let ncontent;
</script>

{#if isAdmin}
  <Card style="margin-bottom: 1.5em;">
    <CardHeader>
      <CardTitle class="mb-1">Admin Options</CardTitle>
    </CardHeader>
    <AdminSettings {ncontent}/>
  </Card>
{/if}

{#if isSupport || isAdmin}
  <Card style="margin-bottom: 1.5em;">
    <CardHeader>
      <CardTitle class="mb-1">Support Options</CardTitle>
    </CardHeader>
    <SupportSettings/>
  </Card>
{/if}

<Card>
  <CardHeader>
    <CardTitle class="mb-1">User Options</CardTitle>
  </CardHeader>
  <UserSettings {username} {isApi}/>
</Card>
