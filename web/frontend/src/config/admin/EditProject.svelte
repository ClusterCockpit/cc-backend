<!--
    @component User managed project edit form card

    Events:
    - `reload`: Trigger upstream reload of user list after project update
 -->

<script>
  import { Card, CardTitle, CardBody } from "@sveltestrap/sveltestrap";
  import { fade } from "svelte/transition";

  /* Svelte 5 Props */
  let { reloadUser } = $props();

  /* State Init */
  let message = $state({ msg: "", color: "#d63384" });
  let displayMessage = $state(false);

  /* Functions */
  async function handleAddProject(event) {
    event.preventDefault();

    const username = document.querySelector("#project-username").value;
    const project = document.querySelector("#project-id").value;

    if (username == "" || project == "") {
      alert("Please fill in a username and select a project.");
      return;
    }

    let formData = new FormData();
    formData.append("username", username);
    formData.append("add-project", project);

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

  async function handleRemoveProject(event) {
    event.preventDefault();

    const username = document.querySelector("#project-username").value;
    const project = document.querySelector("#project-id").value;

    if (username == "" || project == "") {
      alert("Please fill in a username and select a project.");
      return;
    }

    let formData = new FormData();
    formData.append("username", username);
    formData.append("remove-project", project);

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
    <CardTitle class="mb-3"
      >Edit Project Managed By User (Manager Only)</CardTitle
    >
    <div class="input-group mb-3">
      <input
        type="text"
        class="form-control"
        placeholder="username"
        id="project-username"
      />
      <input
        type="text"
        class="form-control"
        placeholder="project-id"
        id="project-id"
      />
      <button
        class="btn btn-primary"
        type="button"
        id="add-project-button"
        onclick={(e) => handleAddProject(e)}>Add</button
      >
      <button
        class="btn btn-danger"
        type="button"
        id="remove-project-button"
        onclick={(e) => handleRemoveProject(e)}>Remove</button
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
