<script>
  import { Card, CardTitle, CardBody } from "@sveltestrap/sveltestrap";
  import { createEventDispatcher } from "svelte";
  import { fade } from "svelte/transition";

  const dispatch = createEventDispatcher();

  let message = { msg: "", color: "#d63384" };
  let displayMessage = false;

  async function handleAddProject() {
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
      const res = await fetch(`/api/user/${username}`, {
        method: "POST",
        body: formData,
      });
      if (res.ok) {
        let text = await res.text();
        popMessage(text, "#048109");
        reloadUserList();
      } else {
        let text = await res.text();
        // console.log(res.statusText)
        throw new Error("Response Code " + res.status + "-> " + text);
      }
    } catch (err) {
      popMessage(err, "#d63384");
    }
  }

  async function handleRemoveProject() {
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
      const res = await fetch(`/api/user/${username}`, {
        method: "POST",
        body: formData,
      });
      if (res.ok) {
        let text = await res.text();
        popMessage(text, "#048109");
        reloadUserList();
      } else {
        let text = await res.text();
        // console.log(res.statusText)
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

  function reloadUserList() {
    dispatch("reload");
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
      <!-- PreventDefault on Sveltestrap-Button more complex to achieve than just use good ol' html button -->
      <!-- see: https://stackoverflow.com/questions/69630422/svelte-how-to-use-event-modifiers-in-my-own-components -->
      <button
        class="btn btn-primary"
        type="button"
        id="add-project-button"
        on:click|preventDefault={handleAddProject}>Add</button
      >
      <button
        class="btn btn-danger"
        type="button"
        id="remove-project-button"
        on:click|preventDefault={handleRemoveProject}>Remove</button
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
