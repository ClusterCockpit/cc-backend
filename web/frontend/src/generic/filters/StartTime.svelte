<!--
    @component Filter sub-component for selecting job starttime

    Properties:
    - `isModified Bool?`: Is this filter component modified [Default: false]
    - `isOpen Bool?`: Is this filter component opened [Default: false]
    - `from Object?`: The currently selected from startime [Default: null]
    - `to Object?`: The currently selected to starttime (i.e. subCluster) [Default: null]
    - `range String?`: The currently selected starttime range as string [Default: ""]

    Events:
    - `set-filter, {String?, String?}`: Set 'from, to' filter in upstream component
 -->

<script>
  import { createEventDispatcher } from "svelte";
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

  const dispatch = createEventDispatcher();

  export let isModified = false;
  export let isOpen = false;
  export let from = null;
  export let to = null;
  export let range = "";
  export let startTimeSelectOptions;

  const now = new Date(Date.now());
  const ago = sub(now, { months: 1 });
  const defaultFrom = {
    date: format(ago, "yyyy-MM-dd"),
    time: format(ago, "HH:mm"),
  };
  const defaultTo = {
    date: format(now, "yyyy-MM-dd"),
    time: format(now, "HH:mm"),
  };

  $: pendingFrom = (from == null) ? defaultFrom : fromRFC3339(from)
  $: pendingTo = (to == null) ? defaultTo : fromRFC3339(to)
  $: pendingRange = range

  $: isModified =
    (from != toRFC3339(pendingFrom) || to != toRFC3339(pendingTo, "59")) &&
    (range != pendingRange) &&
    !(
      from == null &&
      pendingFrom.date == "0000-00-00" &&
      pendingFrom.time == "00:00"
    ) &&
    !(
      to == null &&
      pendingTo.date == "0000-00-00" &&
      pendingTo.time == "00:00"
    ) &&
    !( range == "" && pendingRange == "");

  function toRFC3339({ date, time }, secs = "00") {
    const parsedDate = parse(
      date + " " + time + ":" + secs,
      "yyyy-MM-dd HH:mm:ss",
      new Date(),
    );
    return parsedDate.toISOString();
  }

  function fromRFC3339(rfc3339) {
    const parsedDate = new Date(rfc3339);
    return {
      date: format(parsedDate, "yyyy-MM-dd"),
      time: format(parsedDate, "HH:mm"),
    };
  }
</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Select Start Time</ModalHeader>
  <ModalBody>
    {#if range !== ""}
      <h4>Current Range</h4>
      <Row>
        <FormGroup class="col">
          <Input type ="select" bind:value={pendingRange} >
            {#each startTimeSelectOptions as { rangeLabel, range }}
              <option label={rangeLabel} value={range}/>
            {/each}
          </Input>
        </FormGroup>
      </Row>
    {/if}
    <h4>From</h4>
    <Row>
      <FormGroup class="col">
        <Input type="date" bind:value={pendingFrom.date} disabled={pendingRange !== ""}/>
      </FormGroup>
      <FormGroup class="col">
        <Input type="time" bind:value={pendingFrom.time} disabled={pendingRange !== ""}/>
      </FormGroup>
    </Row>
    <h4>To</h4>
    <Row>
      <FormGroup class="col">
        <Input type="date" bind:value={pendingTo.date} disabled={pendingRange !== ""}/>
      </FormGroup>
      <FormGroup class="col">
        <Input type="time" bind:value={pendingTo.time} disabled={pendingRange !== ""}/>
      </FormGroup>
    </Row>
  </ModalBody>
  <ModalFooter>
    {#if pendingRange !== ""}
      <Button
        color="warning"
        disabled={pendingRange === ""}
        on:click={() => {
          pendingRange = ""
        }}
      >
        Reset Range
      </Button>
      <Button
        color="primary"
        disabled={pendingRange === ""}
        on:click={() => {
          isOpen = false;
          from = null;
          to = null;
          range = pendingRange;
          dispatch("set-filter", { from, to, range });
        }}
      >
        Close & Apply Range
      </Button>
    {:else}
      <Button
        color="primary"
        disabled={pendingFrom.date == "0000-00-00" ||
          pendingTo.date == "0000-00-00"}
        on:click={() => {
          isOpen = false;
          from = toRFC3339(pendingFrom);
          to = toRFC3339(pendingTo, "59");
          range = "";
          dispatch("set-filter", { from, to, range });
        }}
      >
        Close & Apply Dates
      </Button>
    {/if}
    <Button
      color="danger"
      on:click={() => {
        isOpen = false;
        from = null;
        to = null;
        range = "";
        dispatch("set-filter", { from, to, range });
      }}>Reset</Button
    >
    <Button on:click={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
