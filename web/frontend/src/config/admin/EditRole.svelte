<!--
    @component User role edit form card

    Properties:
    - `roles [String]!`: List of roles used in app as strings

    Events:
    - `reload`: Trigger upstream reload of user list after role edit
 -->

<script>
  import { Card, CardTitle, CardBody } from "@sveltestrap/sveltestrap";
  import { fade } from "svelte/transition";

  /* SVelte 5 Props */
  let {roles, reloadUser } = $props();

  /* State Init */
  let message = $state({ msg: "", color: "#d63384" });
  let displayMessage = $state(false);

  /* Functions */
  async function handleAddRole(event) {
    event.preventDefault();

    const username = document.querySelector("#role-username").value;
    const role = document.querySelector("#role-select").value;

    if (username == "" || role == "") {
      alert("Please fill in a username and select a role.");
      return;
    }

    let formData = new FormData();
    formData.append("username", username);
    formData.append("add-role", role);

    try {
      const res = await fetch(`/config/user/${username}`, {
        method: "POST",
        body: formData,
      });
      if (res.ok) {
        let text = await res.text();
        popMessage(text, "#048109");
        reloadUser();
      } else {
        let text = await res.text();
        throw new Error("Response Code " + res.status + "-> " + text);
      }
    } catch (err) {
      popMessage(err, "#d63384");
    }
  }

  async function handleRemoveRole(event) {
    event.preventDefault();

    const username = document.querySelector("#role-username").value;
    const role = document.querySelector("#role-select").value;

    if (username == "" || role == "") {
      alert("Please fill in a username and select a role.");
      return;
    }

    let formData = new FormData();
    formData.append("username", username);
    formData.append("remove-role", role);

    try {
      const res = await fetch(`/config/user/${username}`, {
        method: "POST",
        body: formData,
      });
      if (res.ok) {
        let text = await res.text();
        popMessage(text, "#048109");
        reloadUser();
      } else {
        let text = await res.text();
        throw new Error("Response Code " + res.status + "-> " + text);
      }
    } catch (err) {
      popMessage(err, "#d63384");
    }
  }

  function popMessage(response, rescolor) {
    message = { msg: response, color: rescolor };
    displayMessage = true;
    setTimeout(function () {
      displayMessage = false;
    }, 3500);
  }
</script>

<Card>
  <CardBody>
    <CardTitle class="mb-3">Edit User Roles</CardTitle>
    <div class="input-group mb-3">
      <input
        type="text"
        class="form-control"
        placeholder="username"
        id="role-username"
      />
      <select class="form-select" id="role-select">
        <option selected value="">Role...</option>
        {#each roles as role}
          <option value={role}
            >{role.charAt(0).toUpperCase() + role.slice(1)}</option
          >
        {/each}
      </select>
      <button
        class="btn btn-primary"
        type="button"
        id="add-role-button"
        onclick={(e) => handleAddRole(e)}>Add</button
      >
      <button
        class="btn btn-danger"
        type="button"
        id="remove-role-button"
        onclick={(e) =>handleRemoveRole(e)}>Remove</button
      >
    </div>
    <p>
      {#if displayMessage}<b
          ><code style="color: {message.color};" out:fade
            >Update: {message.msg}</code
          ></b
        >{/if}
    </p>
  </CardBody>
</Card>
