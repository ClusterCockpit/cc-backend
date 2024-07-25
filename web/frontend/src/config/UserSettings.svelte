<!--
    @component User settings wrapper

    Properties:
    - `username String!`: Empty string if auth. is disabled, otherwise the username as string
    - `isApi Bool!`: Is currently logged in user api authority
 -->

<script>
  import { getContext } from "svelte";
  import UserOptions from "./user/UserOptions.svelte";
  import PlotRenderOptions from "./user/PlotRenderOptions.svelte";
  import PlotColorScheme from "./user/PlotColorScheme.svelte";

  export let username
  export let isApi

  const ccconfig = getContext("cc-config");
  let message = { msg: "", target: "", color: "#d63384" };
  let displayMessage = false;

  async function handleSettingSubmit(event) {
    const selector = event.detail.selector
    const target = event.detail.target
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

<UserOptions config={ccconfig} {username} {isApi} bind:message bind:displayMessage on:update-config={(e) => handleSettingSubmit(e)}/>
<PlotRenderOptions config={ccconfig} bind:message bind:displayMessage on:update-config={(e) => handleSettingSubmit(e)}/>
<PlotColorScheme config={ccconfig} bind:message bind:displayMessage on:update-config={(e) => handleSettingSubmit(e)}/>
