<script>
  import {
    Button,
    Table,
    Row,
    Col,
    Card,
    CardTitle,
  } from "@sveltestrap/sveltestrap";
  import { fade } from "svelte/transition";

  export let config;

  let message = { msg: "", target: "", color: "#d63384" };
  let displayMessage = false;

  const colorschemes = {
    Default: [
      "#00bfff",
      "#0000ff",
      "#ff00ff",
      "#ff0000",
      "#ff8000",
      "#ffff00",
      "#80ff00",
    ],
    Autumn: [
      "rgb(255,0,0)",
      "rgb(255,11,0)",
      "rgb(255,20,0)",
      "rgb(255,30,0)",
      "rgb(255,41,0)",
      "rgb(255,50,0)",
      "rgb(255,60,0)",
      "rgb(255,71,0)",
      "rgb(255,80,0)",
      "rgb(255,90,0)",
      "rgb(255,101,0)",
      "rgb(255,111,0)",
      "rgb(255,120,0)",
      "rgb(255,131,0)",
      "rgb(255,141,0)",
      "rgb(255,150,0)",
      "rgb(255,161,0)",
      "rgb(255,171,0)",
      "rgb(255,180,0)",
      "rgb(255,190,0)",
      "rgb(255,201,0)",
      "rgb(255,210,0)",
      "rgb(255,220,0)",
      "rgb(255,231,0)",
      "rgb(255,240,0)",
      "rgb(255,250,0)",
    ],
    Beach: [
      "rgb(0,252,0)",
      "rgb(0,233,0)",
      "rgb(0,212,0)",
      "rgb(0,189,0)",
      "rgb(0,169,0)",
      "rgb(0,148,0)",
      "rgb(0,129,4)",
      "rgb(0,145,46)",
      "rgb(0,162,90)",
      "rgb(0,180,132)",
      "rgb(29,143,136)",
      "rgb(73,88,136)",
      "rgb(115,32,136)",
      "rgb(81,9,64)",
      "rgb(124,51,23)",
      "rgb(162,90,0)",
      "rgb(194,132,0)",
      "rgb(220,171,0)",
      "rgb(231,213,0)",
      "rgb(0,0,13)",
      "rgb(0,0,55)",
      "rgb(0,0,92)",
      "rgb(0,0,127)",
      "rgb(0,0,159)",
      "rgb(0,0,196)",
      "rgb(0,0,233)",
    ],
    BlueRed: [
      "rgb(0,0,131)",
      "rgb(0,0,168)",
      "rgb(0,0,208)",
      "rgb(0,0,247)",
      "rgb(0,27,255)",
      "rgb(0,67,255)",
      "rgb(0,108,255)",
      "rgb(0,148,255)",
      "rgb(0,187,255)",
      "rgb(0,227,255)",
      "rgb(8,255,247)",
      "rgb(48,255,208)",
      "rgb(87,255,168)",
      "rgb(127,255,127)",
      "rgb(168,255,87)",
      "rgb(208,255,48)",
      "rgb(247,255,8)",
      "rgb(255,224,0)",
      "rgb(255,183,0)",
      "rgb(255,143,0)",
      "rgb(255,104,0)",
      "rgb(255,64,0)",
      "rgb(255,23,0)",
      "rgb(238,0,0)",
      "rgb(194,0,0)",
      "rgb(150,0,0)",
    ],
    Rainbow: [
      "rgb(125,0,255)",
      "rgb(85,0,255)",
      "rgb(39,0,255)",
      "rgb(0,6,255)",
      "rgb(0,51,255)",
      "rgb(0,97,255)",
      "rgb(0,141,255)",
      "rgb(0,187,255)",
      "rgb(0,231,255)",
      "rgb(0,255,233)",
      "rgb(0,255,189)",
      "rgb(0,255,143)",
      "rgb(0,255,99)",
      "rgb(0,255,53)",
      "rgb(0,255,9)",
      "rgb(37,255,0)",
      "rgb(83,255,0)",
      "rgb(127,255,0)",
      "rgb(173,255,0)",
      "rgb(217,255,0)",
      "rgb(255,248,0)",
      "rgb(255,203,0)",
      "rgb(255,159,0)",
      "rgb(255,113,0)",
      "rgb(255,69,0)",
      "rgb(255,23,0)",
    ],
    Binary: [
      "rgb(215,215,215)",
      "rgb(206,206,206)",
      "rgb(196,196,196)",
      "rgb(185,185,185)",
      "rgb(176,176,176)",
      "rgb(166,166,166)",
      "rgb(155,155,155)",
      "rgb(145,145,145)",
      "rgb(136,136,136)",
      "rgb(125,125,125)",
      "rgb(115,115,115)",
      "rgb(106,106,106)",
      "rgb(95,95,95)",
      "rgb(85,85,85)",
      "rgb(76,76,76)",
      "rgb(66,66,66)",
      "rgb(55,55,55)",
      "rgb(46,46,46)",
      "rgb(36,36,36)",
      "rgb(25,25,25)",
      "rgb(16,16,16)",
      "rgb(6,6,6)",
    ],
    GistEarth: [
      "rgb(0,0,0)",
      "rgb(2,7,117)",
      "rgb(9,30,118)",
      "rgb(16,53,120)",
      "rgb(23,73,122)",
      "rgb(31,93,124)",
      "rgb(39,110,125)",
      "rgb(47,126,127)",
      "rgb(51,133,119)",
      "rgb(57,138,106)",
      "rgb(62,145,94)",
      "rgb(66,150,82)",
      "rgb(74,157,71)",
      "rgb(97,162,77)",
      "rgb(121,168,83)",
      "rgb(136,173,85)",
      "rgb(153,176,88)",
      "rgb(170,180,92)",
      "rgb(185,182,94)",
      "rgb(189,173,99)",
      "rgb(192,164,101)",
      "rgb(203,169,124)",
      "rgb(215,178,149)",
      "rgb(226,192,176)",
      "rgb(238,212,204)",
      "rgb(248,236,236)",
    ],
    BlueWaves: [
      "rgb(83,0,215)",
      "rgb(43,6,108)",
      "rgb(9,16,16)",
      "rgb(8,32,25)",
      "rgb(0,50,8)",
      "rgb(27,64,66)",
      "rgb(69,67,178)",
      "rgb(115,62,210)",
      "rgb(155,50,104)",
      "rgb(178,43,41)",
      "rgb(180,51,34)",
      "rgb(161,78,87)",
      "rgb(124,117,187)",
      "rgb(78,155,203)",
      "rgb(34,178,85)",
      "rgb(4,176,2)",
      "rgb(9,152,27)",
      "rgb(4,118,2)",
      "rgb(34,92,85)",
      "rgb(78,92,203)",
      "rgb(124,127,187)",
      "rgb(161,187,87)",
      "rgb(180,248,34)",
      "rgb(178,220,41)",
      "rgb(155,217,104)",
      "rgb(115,254,210)",
    ],
    BlueGreenRedYellow: [
      "rgb(0,0,0)",
      "rgb(0,0,20)",
      "rgb(0,0,41)",
      "rgb(0,0,62)",
      "rgb(0,25,83)",
      "rgb(0,57,101)",
      "rgb(0,87,101)",
      "rgb(0,118,101)",
      "rgb(0,150,101)",
      "rgb(0,150,69)",
      "rgb(0,148,37)",
      "rgb(0,141,6)",
      "rgb(60,120,0)",
      "rgb(131,87,0)",
      "rgb(180,25,0)",
      "rgb(203,13,0)",
      "rgb(208,36,0)",
      "rgb(213,60,0)",
      "rgb(219,83,0)",
      "rgb(224,106,0)",
      "rgb(229,129,0)",
      "rgb(233,152,0)",
      "rgb(238,176,0)",
      "rgb(243,199,0)",
      "rgb(248,222,0)",
      "rgb(254,245,0)",
    ],
  };

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
        // console.log(res.statusText)
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

<Row cols={4} class="p-2 g-2">
  <!-- LINE WIDTH -->
  <Col
    ><Card class="h-100">
      <!-- Important: Function with arguments needs to be event-triggered like on:submit={() => functionName('Some','Args')} OR no arguments and like this: on:submit={functionName} -->
      <form
        id="line-width-form"
        method="post"
        action="/api/configuration/"
        class="card-body"
        on:submit|preventDefault={() =>
          handleSettingSubmit("#line-width-form", "lw")}
      >
        <!-- Svelte 'class' directive only on DOMs directly, normal 'class="xxx"' does not work, so style-array it is. -->
        <CardTitle
          style="margin-bottom: 1em; display: flex; align-items: center;"
        >
          <div>Line Width</div>
          <!-- Expand If-Clause for clarity once -->
          {#if displayMessage && message.target == "lw"}
            <div style="margin-left: auto; font-size: 0.9em;">
              <code style="color: {message.color};" out:fade>
                Update: {message.msg}
              </code>
            </div>
          {/if}
        </CardTitle>
        <input type="hidden" name="key" value="plot_general_lineWidth" />
        <div class="mb-3">
          <label for="value" class="form-label">Line Width</label>
          <input
            type="number"
            class="form-control"
            id="lwvalue"
            name="value"
            aria-describedby="lineWidthHelp"
            value={config.plot_general_lineWidth}
            min="1"
          />
          <div id="lineWidthHelp" class="form-text">
            Width of the lines in the timeseries plots.
          </div>
        </div>
        <Button color="primary" type="submit">Submit</Button>
      </form>
    </Card></Col
  >

  <!-- PLOTS PER ROW -->
  <Col
    ><Card class="h-100">
      <form
        id="plots-per-row-form"
        method="post"
        action="/api/configuration/"
        class="card-body"
        on:submit|preventDefault={() =>
          handleSettingSubmit("#plots-per-row-form", "ppr")}
      >
        <!-- Svelte 'class' directive only on DOMs directly, normal 'class="xxx"' does not work, so style-array it is. -->
        <CardTitle
          style="margin-bottom: 1em; display: flex; align-items: center;"
        >
          <div>Plots per Row</div>
          {#if displayMessage && message.target == "ppr"}<div
              style="margin-left: auto; font-size: 0.9em;"
            >
              <code style="color: {message.color};" out:fade
                >Update: {message.msg}</code
              >
            </div>{/if}
        </CardTitle>
        <input type="hidden" name="key" value="plot_view_plotsPerRow" />
        <div class="mb-3">
          <label for="value" class="form-label">Plots per Row</label>
          <input
            type="number"
            class="form-control"
            id="pprvalue"
            name="value"
            aria-describedby="plotsperrowHelp"
            value={config.plot_view_plotsPerRow}
            min="1"
          />
          <div id="plotsperrowHelp" class="form-text">
            How many plots to show next to each other on pages such as
            /monitoring/job/, /monitoring/system/...
          </div>
        </div>
        <Button color="primary" type="submit">Submit</Button>
      </form>
    </Card></Col
  >

  <!-- BACKGROUND -->
  <Col
    ><Card class="h-100">
      <form
        id="backgrounds-form"
        method="post"
        action="/api/configuration/"
        class="card-body"
        on:submit|preventDefault={() =>
          handleSettingSubmit("#backgrounds-form", "bg")}
      >
        <!-- Svelte 'class' directive only on DOMs directly, normal 'class="xxx"' does not work, so style-array it is. -->
        <CardTitle
          style="margin-bottom: 1em; display: flex; align-items: center;"
        >
          <div>Colored Backgrounds</div>
          {#if displayMessage && message.target == "bg"}<div
              style="margin-left: auto; font-size: 0.9em;"
            >
              <code style="color: {message.color};" out:fade
                >Update: {message.msg}</code
              >
            </div>{/if}
        </CardTitle>
        <input type="hidden" name="key" value="plot_general_colorBackground" />
        <div class="mb-3">
          <div>
            {#if config.plot_general_colorBackground}
              <input type="radio" id="true" name="value" value="true" checked />
            {:else}
              <input type="radio" id="true" name="value" value="true" />
            {/if}
            <label for="true">Yes</label>
          </div>
          <div>
            {#if config.plot_general_colorBackground}
              <input type="radio" id="false" name="value" value="false" />
            {:else}
              <input
                type="radio"
                id="false"
                name="value"
                value="false"
                checked
              />
            {/if}
            <label for="false">No</label>
          </div>
        </div>
        <Button color="primary" type="submit">Submit</Button>
      </form>
    </Card></Col
  >

  <!-- PAGING -->
  <Col
    ><Card class="h-100">
      <form
        id="paging-form"
        method="post"
        action="/api/configuration/"
        class="card-body"
        on:submit|preventDefault={() =>
          handleSettingSubmit("#paging-form", "pag")}
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
              <input type="radio" id="true" name="value" value="true" checked />
            {:else}
              <input type="radio" id="true" name="value" value="true" />
            {/if}
            <label for="true">Paging with selectable count of jobs.</label>
          </div>
          <div>
            {#if config.job_list_usePaging}
              <input type="radio" id="false" name="value" value="false" />
            {:else}
              <input
                type="radio"
                id="false"
                name="value"
                value="false"
                checked
              />
            {/if}
            <label for="false">Continuous scroll iteratively adding 10 jobs.</label>
          </div>
        </div>
        <Button color="primary" type="submit">Submit</Button>
      </form>
    </Card></Col
  >
</Row>

<Row cols={1} class="p-2 g-2">
  <!-- COLORSCHEME -->
  <Col
    ><Card>
      <form
        id="colorscheme-form"
        method="post"
        action="/api/configuration/"
        class="card-body"
      >
        <!-- Svelte 'class' directive only on DOMs directly, normal 'class="xxx"' does not work, so style-array it is. -->
        <CardTitle
          style="margin-bottom: 1em; display: flex; align-items: center;"
        >
          <div>Color Scheme for Timeseries Plots</div>
          {#if displayMessage && message.target == "cs"}<div
              style="margin-left: auto; font-size: 0.9em;"
            >
              <code style="color: {message.color};" out:fade
                >Update: {message.msg}</code
              >
            </div>{/if}
        </CardTitle>
        <input type="hidden" name="key" value="plot_general_colorscheme" />
        <Table hover>
          <tbody>
            {#each Object.entries(colorschemes) as [name, rgbrow]}
              <tr>
                <th scope="col">{name}</th>
                <td>
                  {#if rgbrow.join(",") == config.plot_general_colorscheme}
                    <input
                      type="radio"
                      name="value"
                      value={JSON.stringify(rgbrow)}
                      checked
                      on:click={() =>
                        handleSettingSubmit("#colorscheme-form", "cs")}
                    />
                  {:else}
                    <input
                      type="radio"
                      name="value"
                      value={JSON.stringify(rgbrow)}
                      on:click={() =>
                        handleSettingSubmit("#colorscheme-form", "cs")}
                    />
                  {/if}
                </td>
                <td>
                  {#each rgbrow as rgb}
                    <span class="color-dot" style="background-color: {rgb};"
                    ></span>
                  {/each}
                </td>
              </tr>
            {/each}
          </tbody>
        </Table>
      </form>
    </Card></Col
  >
</Row>

<style>
  .color-dot {
    height: 10px;
    width: 10px;
    border-radius: 50%;
    display: inline-block;
  }
</style>
