<!--
    @component Support option select card
 -->

<script>
  import { Row, Col, Card, CardTitle, Button} from "@sveltestrap/sveltestrap";
  import { fade } from "svelte/transition";

  export let config;

  let message;
  let displayMessage;

  async function handleSettingSubmit(selector, target) {
    let form = document.querySelector(selector);
    let formData = new FormData(form);
    try {
      const res = await fetch(form.action, { method: "POST", body: formData });
      if (res.ok) {
        let text = await res.text();
        popMessage(text, target, "#048109");
      } else {
        let text = await res.text();
        throw new Error("Response Code " + res.status + "-> " + text);
      }
    } catch (err) {
      popMessage(err, target, "#d63384");
    }

    return false;
  }

  function popMessage(response, restarget, rescolor) {
    message = { msg: response, target: restarget, color: rescolor };
    displayMessage = true;
    setTimeout(function () {
      displayMessage = false;
    }, 3500);
  }
</script>

<Row cols={1} class="p-2 g-2">
  <Col>
    <Card class="h-100">
      <form
        id="node-paging-form"
        method="post"
        action="/frontend/configuration/"
        class="card-body"
        on:submit|preventDefault={() =>
          handleSettingSubmit("#node-paging-form", "npag")}
      >
        <!-- Svelte 'class' directive only on DOMs directly, normal 'class="xxx"' does not work, so style-array it is. -->
        <CardTitle
          style="margin-bottom: 1em; display: flex; align-items: center;"
        >
          <div>Node List Paging Type</div>
          {#if displayMessage && message.target == "npag"}<div
              style="margin-left: auto; font-size: 0.9em;"
            >
              <code style="color: {message.color};" out:fade
                >Update: {message.msg}</code
              >
            </div>{/if}
        </CardTitle>
        <input type="hidden" name="key" value="node_list_usePaging" />
        <div class="mb-3">
          <div>
            {#if config?.node_list_usePaging}
              <input type="radio" id="nodes-true-checked" name="value" value="true" checked />
            {:else}
              <input type="radio" id="nodes-true" name="value" value="true" />
            {/if}
            <label for="true">Paging with selectable count of nodes.</label>
          </div>
          <div>
            {#if config?.node_list_usePaging}
              <input type="radio" id="nodes-false" name="value" value="false" />
            {:else}
              <input type="radio" id="nodes-false-checked" name="value" value="false" checked />
            {/if}
            <label for="false">Continuous scroll iteratively adding 10 nodes.</label>
          </div>
        </div>
        <Button color="primary" type="submit">Submit</Button>
      </form>
    </Card>
  </Col>
</Row>