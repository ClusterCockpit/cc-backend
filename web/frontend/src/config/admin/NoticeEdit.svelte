<!--
  @component Admin edit notice content card

  Properties:
  - `ncontent String`: The homepage notice content
-->

<script>
  import { Col, Card, CardTitle, CardBody } from "@sveltestrap/sveltestrap";
  import { fade } from "svelte/transition";

  /* Svelte 5 Props */
  let {
    ncontent
  } = $props();

  /* State Init */
  let message = $state({ msg: "", color: "#d63384" });
  let displayMessage = $state(false);

  /* Functions */
  async function handleEditNotice(event) {
    event.preventDefault();

    const content = document.querySelector("#notice-content").value;
    let formData = new FormData();
    formData.append("new-content", content);

    try {
      const res = await fetch(`/config/notice/`, {
        method: "POST",
        body: formData,
      });
      if (res.ok) {
        let text = await res.text();
        popMessage(text, "#048109");
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

<Col>
  <Card class="h-100">
    <CardBody>
      <CardTitle class="mb-3">Edit Notice Shown On Homepage</CardTitle>
      <p>Empty content ("No Content.") hides notice card on homepage.</p>
      <div class="input-group mb-3">
        <input
          type="text"
          class="form-control"
          placeholder="No Content."
          value={ncontent}
          id="notice-content"
        />
        <button
          class="btn btn-primary"
          type="button"
          id="edit-notice-button"
          onclick={(e) => handleEditNotice(e)}>Edit Notice</button
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
</Col>
