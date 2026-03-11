<!--
  @component Admin card for running individual job taggers on all jobs
-->

<script>
  import {
    Col,
    Card,
    CardTitle,
    CardBody,
    Spinner,
    Badge,
  } from "@sveltestrap/sveltestrap";
  import { fade } from "svelte/transition";
  import { onMount, onDestroy } from "svelte";

  /* State Init */
  let taggers = $state([]);
  let message = $state({ msg: "", color: "#d63384" });
  let displayMessage = $state(false);
  let pollTimer = $state(null);

  /* Functions */
  async function fetchTaggers() {
    try {
      const res = await fetch("/config/taggers/");
      if (res.ok) {
        taggers = await res.json();
      }
    } catch (err) {
      console.error("Failed to fetch taggers:", err);
    }
  }

  async function runTagger(name) {
    let formData = new FormData();
    formData.append("name", name);

    try {
      const res = await fetch("/config/taggers/run/", {
        method: "POST",
        body: formData,
      });
      if (res.ok) {
        let text = await res.text();
        popMessage(text, "#048109");
        startPolling();
        await fetchTaggers();
      } else {
        let text = await res.text();
        throw new Error("Response Code " + res.status + " -> " + text);
      }
    } catch (err) {
      popMessage(err, "#d63384");
    }
  }

  function startPolling() {
    if (pollTimer) return;
    pollTimer = setInterval(async () => {
      await fetchTaggers();
      const anyRunning = taggers.some((t) => t.running);
      if (!anyRunning) {
        clearInterval(pollTimer);
        pollTimer = null;
      }
    }, 3000);
  }

  function popMessage(response, rescolor) {
    message = { msg: response, color: rescolor };
    displayMessage = true;
    setTimeout(function () {
      displayMessage = false;
    }, 3500);
  }

  /* Lifecycle */
  onMount(async () => {
    await fetchTaggers();
    const anyRunning = taggers.some((t) => t.running);
    if (anyRunning) startPolling();
  });

  onDestroy(() => {
    if (pollTimer) clearInterval(pollTimer);
  });
</script>

<Col>
  <Card class="h-100">
    <CardBody>
      <CardTitle class="mb-3">Job Taggers</CardTitle>
      <p>Run individual taggers on all existing jobs.</p>
      {#if taggers.length === 0}
        <p class="text-muted">No taggers available.</p>
      {:else}
        <table class="table table-sm mb-3">
          <thead>
            <tr>
              <th>Name</th>
              <th>Type</th>
              <th>Status</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {#each taggers as tagger}
              <tr>
                <td>{tagger.name}</td>
                <td><Badge color="secondary">{tagger.type}</Badge></td>
                <td>
                  {#if tagger.running}
                    <Spinner size="sm" color="primary" /> Running
                  {:else}
                    <span class="text-muted">Idle</span>
                  {/if}
                </td>
                <td>
                  <button
                    class="btn btn-sm btn-primary"
                    disabled={tagger.running}
                    onclick={() => runTagger(tagger.name)}
                  >
                    Run
                  </button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
      <p>
        {#if displayMessage}<b
            ><code style="color: {message.color};" out:fade
              >{message.msg}</code
            ></b
          >{/if}
      </p>
    </CardBody>
  </Card>
</Col>
