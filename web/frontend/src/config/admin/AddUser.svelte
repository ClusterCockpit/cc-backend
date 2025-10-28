<!--
  @component User creation form card

  Properties:
  - `roles [String]!`: List of roles used in app as strings
  - `reloadUser Func`: The callback function to reload the user list
-->

 <script>
  import { Button, Card, CardTitle } from "@sveltestrap/sveltestrap";
  import { fade } from "svelte/transition";

  /* Svelte 5 Props */
  let {
    roles,
    reloadUser
  } = $props();

  /* State Init */
  let message = $state({ msg: "", color: "#d63384" });
  let displayMessage = $state(false);

  /* Functions */
  async function handleUserSubmit(event) {
    event.preventDefault();
    
    let form = document.querySelector("#create-user-form");
    let formData = new FormData(form);

    try {
      const res = await fetch(form.action, { method: "POST", body: formData });
      if (res.ok) {
        let text = await res.text();
        popMessage(text, "#048109");
        reloadUser();
        form.reset();
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
  <form
    id="create-user-form"
    method="post"
    action="/config/users/"
    class="card-body"
    autocomplete="off"
    onsubmit={(e) => handleUserSubmit(e)}
  >
    <CardTitle class="mb-3">Create User</CardTitle>
    <div class="mb-3">
      <label for="username" class="form-label">Username (ID)</label>
      <input
        type="text"
        class="form-control"
        id="username"
        name="username"
        aria-describedby="usernameHelp"
        autocomplete="username"
      />
      <div id="usernameHelp" class="form-text">Must be unique.</div>
    </div>
    <div class="mb-3">
      <label for="password" class="form-label">Password</label>
      <input
        type="password"
        class="form-control"
        id="password"
        name="password"
        aria-describedby="passwordHelp"
        autocomplete="new-password"
      />
      <div id="passwordHelp" class="form-text">
        Only API users are allowed to have a blank password. Users with a blank
        password can only authenticate via Tokens.
      </div>
    </div>
    <div class="mb-3">
      <label for="name" class="form-label">Project</label>
      <input
        type="text"
        class="form-control"
        id="project"
        name="project"
        aria-describedby="projectHelp"
      />
      <div id="projectHelp" class="form-text">
        Only Manager users can have a project. Allows to inspect jobs and users
        of given project.
      </div>
    </div>
    <div class="mb-3">
      <label for="name" class="form-label">Name</label>
      <input
        type="text"
        class="form-control"
        id="name"
        name="name"
        aria-describedby="nameHelp"
        autocomplete="name"
      />
      <div id="nameHelp" class="form-text">Optional, can be blank.</div>
    </div>
    <div class="mb-3">
      <label for="email" class="form-label">Email address</label>
      <input
        type="email"
        class="form-control"
        id="email"
        name="email"
        aria-describedby="emailHelp"
        autocomplete="email"
      />
      <div id="emailHelp" class="form-text">Optional, can be blank.</div>
    </div>

    <div class="mb-3">
      <p>Role:</p>
      {#each roles as role, i}
        {#if i == 0}
          <div>
            <input type="radio" id={role} name="role" value={role} checked />
            <label for={role}
              >{role.toUpperCase()} (Allowed to interact with REST API.)</label
            >
          </div>
        {:else if i == 1}
          <div>
            <input type="radio" id={role} name="role" value={role} checked />
            <label for={role}
              >{role.charAt(0).toUpperCase() + role.slice(1)} (Same as if created
              via LDAP sync.)</label
            >
          </div>
        {:else}
          <div>
            <input type="radio" id={role} name="role" value={role} />
            <label for={role}
              >{role.charAt(0).toUpperCase() + role.slice(1)}</label
            >
          </div>
        {/if}
      {/each}
    </div>
    <p style="display: flex; align-items: center;">
      <Button type="submit" color="primary" style="margin-right: 1.5em;">Submit</Button>
      {#if displayMessage}
        <b
          ><code style="color: {message.color};" out:fade>{message.msg}</code
          ></b
        >
      {/if}
    </p>
  </form>
</Card>
