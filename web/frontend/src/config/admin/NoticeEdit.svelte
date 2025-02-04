<!--
    @component Admin edit notice.txt content card
 -->

<script>
  import { Col, Card, CardTitle, CardBody } from "@sveltestrap/sveltestrap";
  import { fade } from "svelte/transition";

  export let ncontent;

  let message = { msg: "", color: "#d63384" };
  let displayMessage = false;

  async function handleEditNotice() {
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

        <!-- PreventDefault on Sveltestrap-Button more complex to achieve than just use good ol' html button -->
        <!-- see: https://stackoverflow.com/questions/69630422/svelte-how-to-use-event-modifiers-in-my-own-components -->
        <button
          class="btn btn-primary"
          type="button"
          id="edit-notice-button"
          on:click|preventDefault={() => handleEditNotice()}>Edit Notice</button
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
