<!--
    @component User management table

    Properties:
    - `users [Object]?`: List of users

    Events:
    - `reload`: Trigger upstream reload of user list
 -->

<script>
  import {
    Button,
    Table,
    Card,
    CardTitle,
    CardBody,
  } from "@sveltestrap/sveltestrap";
  import { createEventDispatcher } from "svelte";
  import ShowUsersRow from "./ShowUsersRow.svelte";

  export let users = [];

  const dispatch = createEventDispatcher();
  function reloadUserList() {
    dispatch("reload");
  }

  function deleteUser(username) {
    if (confirm("Are you sure?")) {
      let formData = new FormData();
      formData.append("username", username);
      fetch("/config/users/", { method: "DELETE", body: formData }).then((res) => {
        if (res.status == 200) {
          reloadUserList();
        } else {
          confirm(res.statusText);
        }
      });
    }
  }

  $: userList = users;
</script>

<Card class="h-100">
  <CardBody>
    <CardTitle class="mb-3">Special Users</CardTitle>
    <p>
      Not created by an LDAP sync and/or having a role other than <code
        >user</code
      >
      <Button
        color="secondary"
        size="sm"
        on:click={reloadUserList}
        style="float: right;">Reload</Button
      >
    </p>
    <div style="width: 100%; max-height: 500px; overflow-y: scroll;">
      <Table hover>
        <thead>
          <tr>
            <th>Username</th>
            <th>Name</th>
            <th>Project(s)</th>
            <th>Email</th>
            <th>Roles</th>
            <th>JWT</th>
            <th>Delete</th>
          </tr>
        </thead>
        <tbody id="users-list">
          {#each userList as user}
            <tr id="user-{user.username}">
              <ShowUsersRow {user} />
              <td
                ><button
                  class="btn btn-danger del-user"
                  on:click={deleteUser(user.username)}>Delete</button
                ></td
              >
            </tr>
          {:else}
            <tr>
              <td colspan="4">
                <div class="spinner-border" role="status">
                  <span class="visually-hidden">Loading...</span>
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </Table>
    </div>
  </CardBody>
</Card>
