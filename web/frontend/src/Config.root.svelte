<!--
  @component Main Config Option Component, Wrapper for admin and user sub-components

  Properties:
  - `ìsAdmin Bool!`: Is currently logged in user admin authority
  - `isSupport Bool!`: Is currently logged in user support authority
  - `isApi Bool!`: Is currently logged in user api authority
  - `username String!`: Empty string if auth. is disabled, otherwise the username as string
  - `ncontent String!`: The currently displayed message on the homescreen
  - `clusterNames [String]`: The available clusternames
-->

<script>
  import { Card, CardHeader, CardTitle } from "@sveltestrap/sveltestrap";
  import UserSettings from "./config/UserSettings.svelte";
  import SupportSettings from "./config/SupportSettings.svelte";
  import AdminSettings from "./config/AdminSettings.svelte";

  /* Svelte 5 Props */
  let {
    isAdmin,
    isSupport,
    isApi,
    username,
    ncontent,
    clusterNames
  } = $props();
</script>

{#if isAdmin}
  <Card style="margin-bottom: 1.5rem;">
    <CardHeader>
      <CardTitle class="mb-1">Admin Options</CardTitle>
    </CardHeader>
    <AdminSettings {ncontent} {clusterNames}/>
  </Card>
{/if}

{#if isSupport || isAdmin}
  <Card style="margin-bottom: 1.5rem;">
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
