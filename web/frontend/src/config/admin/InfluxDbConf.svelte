<script>
  import { Button, Card, CardTitle   } from "sveltestrap";
  import { createEventDispatcher } from "svelte";
  import { fade } from "svelte/transition";

  const dispatch = createEventDispatcher();

  let message = { msg: "", color: "#d63384" };
  let displayMessage = false;

  async function handleInfluxSubmit() {
    let form = document.querySelector("#create-influx-form");
    let formData = new FormData(form);

    try {
      const res = await fetch(form.action, { method: "POST", body: formData });
      if (res.ok) {
        let text = await res.text();
        popMessage(text, "#048109");
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

<Card class="m-2">
  <form
    id="create-influx-form"
    method="post"
    action="/api/influx/"
    class="card-body"
    on:submit|preventDefault={handleInfluxSubmit}
  >
    <!-- <CardTitle class="mb-3">Create Influx Configuration</CardTitle> -->
    <div class="mb-3">
      <label for="type" class="form-label">Type</label>
      <input
        type="text"
        class="form-control"
        id="type"
        name="type"
        value="influxasync"
      />
    </div>
    <div class="mb-3">
      <label for="database" class="form-label">Database</label>
      <input
        type="text"
        class="form-control"
        id="database"
        name="database"
        value="Bucket"
      />
    </div>
    <div class="mb-3">
      <label for="host" class="form-label">Host</label>
      <input
        type="text"
        class="form-control"
        id="host"
        name="host"
        value="103.165.95.123"
      />
    </div>
    <div class="mb-3">
      <label for="port" class="form-label">Port</label>
      <input
        type="text"
        class="form-control"
        id="port"
        name="port"
        value="8086"
      />
    </div>
    <!-- Add more input fields as needed -->
    <div class="mb-3">
      <label for="user" class="form-label">User</label>
      <input type="text" class="form-control" id="user" name="user" value="" />
    </div>
    <div class="mb-3">
      <label for="password" class="form-label">Password</label>
      <input
        type="password"
        class="form-control"
        id="password"
        name="password"
        value="-7snr5tvEWxx5pnNSwfqSz9C2hO6NHUAqMRIBeS3Q-Z8kmPn0pO6UC-IEWo2EAw3oTorriVGWwiNYahS2BTiFg=="
      />
    </div>
    <div class="mb-3">
      <label for="organization" class="form-label">Organization</label>
      <input
        type="text"
        class="form-control"
        id="organization"
        name="organization"
        value="myorg"
      />
    </div>
    <div class="mb-3">
    <label for="ssl" class="form-label">SSL</label>
    <select class="form-control" id="ssl" name="ssl">
        {#each ['true', 'false'] as option}
            <option>{option}</option>
        {/each}
    </select>
    </div>
    <div class="mb-3">
      <label for="batch_size" class="form-label">Batch Size</label>
      <input
        type="number"
        class="form-control"
        id="batch_size"
        name="batch_size"
        value="200"
      />
    </div>
    <div class="mb-3">
      <label for="retry_interval" class="form-label">Retry Interval</label>
      <input
        type="text"
        class="form-control"
        id="retry_interval"
        name="retry_interval"
        value="1s"
      />
    </div>
    <div class="mb-3">
      <label for="retry_exponential_base" class="form-label"
        >Retry Exponential Base</label
      >
      <input
        type="number"
        class="form-control"
        id="retry_exponential_base"
        name="retry_exponential_base"
        value="2"
      />
    </div>
    <div class="mb-3">
      <label for="max_retries" class="form-label">Max Retries</label>
      <input
        type="number"
        class="form-control"
        id="max_retries"
        name="max_retries"
        value="20"
      />
    </div>
    <div class="mb-3">
      <label for="max_retry_time" class="form-label">Max Retry Time</label>
      <input
        type="text"
        class="form-control"
        id="max_retry_time"
        name="max_retry_time"
        value="168h"
      />
    </div>
    <div class="mb-3">
      <label for="meta_as_tags" class="form-label">Meta as Tags</label>
      <input
        type="text"
        class="form-control"
        id="meta_as_tags"
        name="meta_as_tags"
        value="[]"
      />
    </div>
    <p style="display: flex; align-items: center;">
      <Button type="submit" color="primary">Submit</Button>
      {#if displayMessage}<div style="margin-left: 1.5em;">
          <b
            ><code style="color: {message.color};" out:fade>{message.msg}</code
            ></b
          >
        </div>{/if}
    </p>
  </form>
</Card>
