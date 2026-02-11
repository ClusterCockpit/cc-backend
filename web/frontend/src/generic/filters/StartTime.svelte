<!--
  @component Filter sub-component for selecting job starttime

  Properties:
  - `isOpen Bool?`: Is this filter component opened [Bindable, Default: false]
  - `presetStartTime Object?`: Object containing the latest duration filter parameters
    - Default: { from: null, to: null, range: "" }
  - `setFilter Func`: The callback function to apply current filter selection

  Exported:
  - `const startTimeSelectOptions [Object]`: List of available fixed startTimes used in cc-backend
-->

<script module>
  export const startTimeSelectOptions = [
    { range: "", rangeLabel: "No Selection"},
    { range: "last6h", rangeLabel: "Last 6 hrs"},
    { range: "last24h", rangeLabel: "Last 24 hrs"},
    { range: "last7d", rangeLabel: "Last 7 days"},
    { range: "last30d", rangeLabel: "Last 30 days"}
  ];
</script>

<script>
  /* Note: Ignore VSCode reported 'A component can only have one instance-level <script> element' error */
  import { parse, format, sub } from "date-fns";
  import {
    Row,
    Button,
    Input,
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
    FormGroup,
  } from "@sveltestrap/sveltestrap";

  /* Svelte 5 Props */
  let {
    isOpen = $bindable(false),
    presetStartTime = { from: null, to: null, range: "" },
    setFilter
  } = $props();

  /* Const Init */
  const now = new Date(Date.now());
  const ago = sub(now, { months: 1 });
  const resetFrom = { date: format(ago, "yyyy-MM-dd"), time: format(ago, "HH:mm")};
  const resetTo = { date: format(now, "yyyy-MM-dd"), time: format(now, "HH:mm")};

  /* Derived */
  let pendingStartTime = $derived(presetStartTime);
  let fromState = $derived(fromRFC3339(presetStartTime?.from, resetFrom));
  let toState = $derived(fromRFC3339(presetStartTime?.to, resetTo));

  const rangeSelect = $derived(pendingStartTime?.range ? pendingStartTime.range : "")

  /* Functions */
  function fromRFC3339(rfc3339, reset) {
    if (rfc3339) {
      const parsedDate = new Date(rfc3339);
      return {
        date: format(parsedDate, "yyyy-MM-dd"),
        time: format(parsedDate, "HH:mm"),
      }
    } else {
      return reset
    } 
  }

  function toRFC3339({ date, time }, secs = "00") {
    const parsedDate = parse(
      date + " " + time + ":" + secs,
      "yyyy-MM-dd HH:mm:ss",
      new Date(),
    );
    return parsedDate.toISOString();
  }
</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Select Start Time</ModalHeader>
  <ModalBody>
    {#if rangeSelect !== ""}
      <h4>Current Range</h4>
      <Row>
        <FormGroup class="col">
          <Input type ="select" bind:value={pendingStartTime.range} >
            {#each startTimeSelectOptions as { rangeLabel, range }}
              <option label={rangeLabel} value={range}></option>
            {/each}
          </Input>
        </FormGroup>
      </Row>
    {/if}
    <h4>From</h4>
    <Row>
      <FormGroup class="col">
        <Input type="date" bind:value={fromState.date} disabled={rangeSelect !== ""}/>
      </FormGroup>
      <FormGroup class="col">
        <Input type="time" bind:value={fromState.time} disabled={rangeSelect !== ""}/>
      </FormGroup>
    </Row>
    <h4>To</h4>
    <Row>
      <FormGroup class="col">
        <Input type="date" bind:value={toState.date} disabled={rangeSelect !== ""}/>
      </FormGroup>
      <FormGroup class="col">
        <Input type="time" bind:value={toState.time} disabled={rangeSelect !== ""}/>
      </FormGroup>
    </Row>
  </ModalBody>
  <ModalFooter>
    {#if rangeSelect !== ""}
      <Button
        color="warning"
        disabled={rangeSelect === ""}
        onclick={() => {
          pendingStartTime.range = "";
        }}
      >
        Reset Range
      </Button>
      <Button
        color="primary"
        disabled={rangeSelect === ""}
        onclick={() => {
          isOpen = false;
          pendingStartTime.from = null;
          pendingStartTime.to = null;
          setFilter({ startTime: pendingStartTime });
        }}
      >
        Close & Apply Range
      </Button>
    {:else}
      <Button
        color="primary"
        disabled={fromState.date == "0000-00-00" ||
          toState.date == "0000-00-00"}
        onclick={() => {
          isOpen = false;
          pendingStartTime.from = toRFC3339(fromState);
          pendingStartTime.to = toRFC3339(toState, "59");
          pendingStartTime.range = "";
          setFilter({ startTime: pendingStartTime });
        }}
      >
        Close & Apply Dates
      </Button>
    {/if}
    <Button
      color="danger"
      onclick={() => {
        isOpen = false;
        fromState = resetFrom;
        toState = resetTo;
        pendingStartTime.from = null;
        pendingStartTime.to = null;
        pendingStartTime.range = "";
        setFilter({ startTime: pendingStartTime });
      }}>Reset</Button
    >
    <Button onclick={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
