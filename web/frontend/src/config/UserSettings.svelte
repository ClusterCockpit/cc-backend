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

  /* Svelte 5 Props */
  let {
    username,
    isApi
  } = $props();

  /* Const Init */
  const ccconfig = getContext("cc-config");
  
  /* State Init */
  let message = $state({ msg: "", target: "", color: "#d63384" });
  let displayMessage = $state(false);
  let cbmode = $state(ccconfig?.plotConfiguration_colorblindMode || false);

  /* Functions */
  async function handleSettingSubmit(event, setting) {
    event.preventDefault();

    const selector = setting.selector
    const target = setting.target
    let form = document.querySelector(selector);
    let formData = new FormData(form);
    try {
      const res = await fetch(form.action, { method: "POST", body: formData });
      if (res.ok) {
        let text = await res.text();
        if (formData.get("key") === "plotConfiguration_colorblindMode") {
          cbmode = JSON.parse(formData.get("value"));
        }
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

<UserOptions config={ccconfig} {username} {isApi} bind:message bind:displayMessage updateSetting={(e, newSetting) => handleSettingSubmit(e, newSetting)}/>
<PlotRenderOptions config={ccconfig} bind:message bind:displayMessage updateSetting={(e, newSetting) => handleSettingSubmit(e, newSetting)}/>
<PlotColorScheme config={ccconfig} bind:cbmode bind:message bind:displayMessage updateSetting={(e, newSetting) => handleSettingSubmit(e, newSetting)}/>
