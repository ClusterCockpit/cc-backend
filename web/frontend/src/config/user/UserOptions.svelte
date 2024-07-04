<script>
    import {
        Button,
        Row,
        Col,
        Card,
        CardTitle,
        CardBody
    } from "@sveltestrap/sveltestrap";
    import { fade } from "svelte/transition";
    import { createEventDispatcher } from 'svelte';
    import { fetchJwt } from "../../utils.js";

    export let config;
    export let message;
    export let displayMessage;
    export let username;
    export let isApi;

    let jwt = "";
    function getUserJwt(username) {
        if (username) {
            const p = fetchJwt(username);
            p.then((content) => {
                jwt = content
            }).catch((error) => {
                console.error(`Could not get JWT: ${error}`);
            });
        }
    }

    function clipJwt() {
        navigator.clipboard
         .writeText(jwt)
         .catch((reason) => console.error(reason));
    }

    const dispatch = createEventDispatcher();
    function updateSetting(selector, target) {
        dispatch('update', {
            selector: selector,
            target: target
        });
    }
</script>

<Row cols={isApi ? 3 : 1} class="p-2 g-2">
  <!-- PAGING -->
  <Col>
    <Card class="h-100">
      <form
        id="paging-form"
        method="post"
        action="/userconfig/configuration/"
        class="card-body"
        on:submit|preventDefault={() =>
          updateSetting("#paging-form", "pag")}
      >
        <!-- Svelte 'class' directive only on DOMs directly, normal 'class="xxx"' does not work, so style-array it is. -->
        <CardTitle
          style="margin-bottom: 1em; display: flex; align-items: center;"
        >
          <div>Paging Type</div>
          {#if displayMessage && message.target == "pag"}<div
              style="margin-left: auto; font-size: 0.9em;"
            >
              <code style="color: {message.color};" out:fade
                >Update: {message.msg}</code
              >
            </div>{/if}
        </CardTitle>
        <input type="hidden" name="key" value="job_list_usePaging" />
        <div class="mb-3">
          <div>
            {#if config.job_list_usePaging}
              <input type="radio" id="true-checked" name="value" value="true" checked />
            {:else}
              <input type="radio" id="true" name="value" value="true" />
            {/if}
            <label for="true">Paging with selectable count of jobs.</label>
          </div>
          <div>
            {#if config.job_list_usePaging}
              <input type="radio" id="false" name="value" value="false" />
            {:else}
              <input type="radio" id="false-checked" name="value" value="false" checked />
            {/if}
            <label for="false">Continuous scroll iteratively adding 10 jobs.</label>
          </div>
        </div>
        <Button color="primary" type="submit">Submit</Button>
      </form>
    </Card>
  </Col>

  {#if isApi}
    <!-- USER-JWT BTN -->
    <Col>
        <Card class="h-100">
            <CardBody>
                <CardTitle>Generate JWT</CardTitle>
                {#if jwt}
                    <Button color="secondary" on:click={clipJwt()}>
                        Copy JWT to Clipboard
                    </Button>
                    <p class="mt-2">
                        Your token is displayed on the right. Press this button to copy it to the clipboard.
                    </p>
                {:else}
                    <Button color="success" on:click={getUserJwt(username)}>
                        Generate JWT for '{username}'
                    </Button>
                    <p class="mt-2">
                        Generate a JSON Web Token for use with the ClusterCockpit REST-API endpoints.
                    </p>
                {/if}
            </CardBody>
        </Card>
    </Col>

    <!-- USER-JWT RES -->
    <Col>
        <Card class="h-100">
            <CardBody>
                <CardTitle>Display JWT</CardTitle>
                <textarea cols="32" rows="5" readonly>{jwt ? jwt : 'Press "Gen. JWT" to request token ...'}</textarea>
            </CardBody>
        </Card>
    </Col>
  {/if}
</Row>