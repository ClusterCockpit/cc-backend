<!--
  @component Plot render option selection for users

  Properties:
  - `config Object`: Current cc-config
  - `message Object`: Message to display on success or error [Bindable]
  - `displayMessage Bool`: If to display message content [Bindable]
  - `updateSetting Func`: The callback function to apply current option selection
-->

<script>
  import {
    Button,
    Row,
    Col,
    Card,
    CardTitle,
  } from "@sveltestrap/sveltestrap";
  import { fade } from "svelte/transition";

  /* Svelte 5 Props */
  let {
    config,
    message = $bindable(),
    displayMessage = $bindable(),
    updateSetting
  } = $props();
</script>

<Row cols={3} class="p-2 g-2">
  <!-- LINE WIDTH -->
  <Col>
    <Card class="h-100">
      <form
        id="line-width-form"
        method="post"
        action="/frontend/configuration/"
        class="card-body"
        onsubmit={(e) => updateSetting(e, {
          selector: "#line-width-form",
          target: "lw",
        })}
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
    </Card>
  </Col>

  <!-- PLOTS PER ROW -->
  <Col>
    <Card class="h-100">
      <form
        id="plots-per-row-form"
        method="post"
        action="/frontend/configuration/"
        class="card-body"
        onsubmit={(e) => updateSetting(e, {
          selector: "#plots-per-row-form",
          target: "ppr",
        })}
      >
        <!-- Svelte 'class' directive only on DOMs directly, normal 'class="xxx"' does not work, so style-array it is. -->
        <CardTitle
          style="margin-bottom: 1em; display: flex; align-items: center;"
        >
          <div>Plots per Row</div>
          {#if displayMessage && message.target == "ppr"}
            <div style="margin-left: auto; font-size: 0.9em;">
              <code style="color: {message.color};" out:fade>
                Update: {message.msg}
              </code>
            </div>
          {/if}
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
    </Card>
  </Col>

  <!-- BACKGROUND -->
  <Col class="d-flex justify-content-between">
    <Card class="h-100" style="width: 49%;">
      <form
        id="backgrounds-form"
        method="post"
        action="/frontend/configuration/"
        class="card-body"
        onsubmit={(e) => updateSetting(e, {
          selector: "#backgrounds-form",
          target: "bg",
        })}
      >
        <!-- Svelte 'class' directive only on DOMs directly, normal 'class="xxx"' does not work, so style-array it is. -->
        <CardTitle
          style="margin-bottom: 1em; display: flex; align-items: center;"
        >
          <div>Colored Backgrounds</div>
          {#if displayMessage && message.target == "bg"}
            <div style="margin-left: auto; font-size: 0.9em;">
              <code style="color: {message.color};" out:fade>
                Update: {message.msg}
              </code>
            </div>
          {/if}
        </CardTitle>
        <input type="hidden" name="key" value="plot_general_colorBackground" />
        <div class="mb-3">
          <div>
            {#if config.plot_general_colorBackground}
              <input type="radio" id="colb-true-checked" name="value" value="true" checked />
            {:else}
              <input type="radio" id="colb-true" name="value" value="true" />
            {/if}
            <label for="true">Yes</label>
          </div>
          <div>
            {#if config.plot_general_colorBackground}
              <input type="radio" id="colb-false" name="value" value="false" />
            {:else}
              <input type="radio" id="colb-false-checked" name="value" value="false" checked />
            {/if}
            <label for="false">No</label>
          </div>
        </div>
        <Button color="primary" type="submit">Submit</Button>
      </form>
    </Card>
    <Card class="h-100" style="width: 49%;">
      <form
        id="colorblindmode-form"
        method="post"
        action="/frontend/configuration/"
        class="card-body"
        onsubmit={(e) => updateSetting(e, {
          selector: "#colorblindmode-form",
          target: "cbm",
        })}
      >
        <!-- Svelte 'class' directive only on DOMs directly, normal 'class="xxx"' does not work, so style-array it is. -->
        <CardTitle
          style="margin-bottom: 1em; display: flex; align-items: center;"
        >
          <div>Color Blind Mode</div>
          {#if displayMessage && message.target == "cbm"}
            <div style="margin-left: auto; font-size: 0.9em;">
              <code style="color: {message.color};" out:fade>
                Update: {message.msg}
              </code>
            </div>
          {/if}
        </CardTitle>
        <input type="hidden" name="key" value="plot_general_colorblindMode" />
        <div class="mb-3">
          <div>
            {#if config?.plot_general_colorblindMode}
              <input type="radio" id="cbm-true-checked" name="value" value="true" checked />
            {:else}
              <input type="radio" id="cbm-true" name="value" value="true" />
            {/if}
            <label for="true">Yes</label>
          </div>
          <div>
            {#if config?.plot_general_colorblindMode}
              <input type="radio" id="cbm-false" name="value" value="false" />
            {:else}
              <input type="radio" id="cbm-false-checked" name="value" value="false" checked />
            {/if}
            <label for="false">No</label>
          </div>
        </div>
        <Button color="primary" type="submit">Submit</Button>
      </form>
    </Card>
  </Col>
</Row>