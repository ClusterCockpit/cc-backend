<!--
    @component User data row for table

    Properties:
    - `user Object!`: User Object
      - {username: String, name: String, roles: [String], projects: String, email: String}
 -->

<script>
  import { Button } from "@sveltestrap/sveltestrap";
  import { fetchJwt } from "../../generic/utils.js"

  /* Svelte 5 Props */
  let { user } = $props();

  /* State Init */
  let jwt = $state("");

  /* Functions */
  function getUserJwt(username) {
    const p = fetchJwt(username);
    p.then((content) => {
        jwt = content
    }).catch((error) => {
        console.error(`Could not get JWT: ${error}`);
    });
  }
</script>

<td>{user.username}</td>
<td>{user.name}</td>
<td style="max-width: 200px;">{user.projects}</td>
<td>{user.email}</td>
<td><code>{user?.roles ? user.roles.join(", ") : "No Roles"}</code></td>
<td>
  {#if !jwt}
    <Button color="success" onclick={() => getUserJwt(user.username)}
      >Gen. JWT</Button
    >
  {:else}
    <textarea rows="3" cols="20">{jwt}</textarea>
  {/if}
</td>
