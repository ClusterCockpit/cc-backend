<!--
  @component Filter sub-component for selecting job duration

  Properties:
  - `isOpen Bool?`: Is this filter component opened [Bindable, Default: false]
  - `presetDuration Object?`: Object containing the latest duration filter parameters
    - Default: { lessThan: null, moreThan: null, from: null, to: null }
  - `setFilter Func`: The callback function to apply current filter selection
-->
 
 <script>
  import {
    Row,
    Col,
    Button,
    Modal,
    ModalBody,
    ModalHeader,
    ModalFooter,
  } from "@sveltestrap/sveltestrap";

  /* Svelte 5 Props */
  let {
    isOpen = $bindable(false),
    presetDuration = {
      lessThan: null,
      moreThan: null,
      from: null,
      to: null
    },
    setFilter
  } = $props();

  /* Derived */
  let pendingDuration = $derived(presetDuration);
  let lessState = $derived(secsToHoursAndMins(presetDuration?.lessThan));
  let moreState = $derived(secsToHoursAndMins(presetDuration?.moreThan));
  let fromState = $derived(secsToHoursAndMins(presetDuration?.from));
  let toState = $derived(secsToHoursAndMins(presetDuration?.to));

  const lessDisabled = $derived(
    moreState.hours !== 0 ||
    moreState.mins !== 0 ||
    fromState.hours !== 0 ||
    fromState.mins !== 0 ||
    toState.hours !== 0 ||
    toState.mins !== 0
  );

  const moreDisabled = $derived(
    lessState.hours !== 0 ||
    lessState.mins !== 0 ||
    fromState.hours !== 0 ||
    fromState.mins !== 0 ||
    toState.hours !== 0 ||
    toState.mins !== 0
  );

  const betweenDisabled = $derived(
    moreState.hours !== 0 ||
    moreState.mins !== 0 ||
    lessState.hours !== 0 ||
    lessState.mins !== 0
  )

  /* Functions */
  function resetPending() {
    pendingDuration = {
      lessThan: null,
      moreThan: null,
      from: null,
      to: null
    }
  };

  function resetStates() {
    lessState = { hours: 0, mins: 0 }
    moreState = { hours: 0, mins: 0 }
    fromState = { hours: 0, mins: 0 }
    toState = { hours: 0, mins: 0 }
  };

  function secsToHoursAndMins(seconds) {
    if (seconds) {
      const hours = Math.floor(seconds / 3600);
      seconds -= hours * 3600;
      const mins = Math.floor(seconds / 60);
      return { hours, mins };
    } else {
      return { hours: 0, mins: 0 }
    }
  }

  function hoursAndMinsToSecs(hoursAndMins) {
    if (hoursAndMins) {
      return hoursAndMins.hours * 3600 + hoursAndMins.mins * 60;
    } else {
      return 0
    }
  }
</script>

<Modal {isOpen} toggle={() => (isOpen = !isOpen)}>
  <ModalHeader>Select Job Duration</ModalHeader>
  <ModalBody>
    <h4>Duration more than</h4>
    <Row>
      <Col>
        <div class="input-group mb-2 mr-sm-2">
          <input
            type="number"
            min="0"
            class="form-control"
            bind:value={moreState.hours}
            disabled={moreDisabled}
          />
          <div class="input-group-append">
            <div class="input-group-text">h</div>
          </div>
        </div>
      </Col>
      <Col>
        <div class="input-group mb-2 mr-sm-2">
          <input
            type="number"
            min="0"
            max="59"
            class="form-control"
            bind:value={moreState.mins}
            disabled={moreDisabled}
          />
          <div class="input-group-append">
            <div class="input-group-text">m</div>
          </div>
        </div>
      </Col>
    </Row>
    <hr />

    <h4>Duration less than</h4>
    <Row>
      <Col>
        <div class="input-group mb-2 mr-sm-2">
          <input
            type="number"
            min="0"
            class="form-control"
            bind:value={lessState.hours}
            disabled={lessDisabled}
          />
          <div class="input-group-append">
            <div class="input-group-text">h</div>
          </div>
        </div>
      </Col>
      <Col>
        <div class="input-group mb-2 mr-sm-2">
          <input
            type="number"
            min="0"
            max="59"
            class="form-control"
            bind:value={lessState.mins}
            disabled={lessDisabled}
          />
          <div class="input-group-append">
            <div class="input-group-text">m</div>
          </div>
        </div>
      </Col>
    </Row>
    <hr />

    <h4>Duration between</h4>
    <Row>
      <Col>
        <div class="input-group mb-2 mr-sm-2">
          <input
            type="number"
            min="0"
            class="form-control"
            bind:value={fromState.hours}
            disabled={betweenDisabled}
          />
          <div class="input-group-append">
            <div class="input-group-text">h</div>
          </div>
        </div>
      </Col>
      <Col>
        <div class="input-group mb-2 mr-sm-2">
          <input
            type="number"
            min="0"
            max="59"
            class="form-control"
            bind:value={fromState.mins}
            disabled={betweenDisabled}
          />
          <div class="input-group-append">
            <div class="input-group-text">m</div>
          </div>
        </div>
      </Col>
    </Row>
    <h4>and</h4>
    <Row>
      <Col>
        <div class="input-group mb-2 mr-sm-2">
          <input
            type="number"
            min="0"
            class="form-control"
            bind:value={toState.hours}
            disabled={betweenDisabled}
          />
          <div class="input-group-append">
            <div class="input-group-text">h</div>
          </div>
        </div>
      </Col>
      <Col>
        <div class="input-group mb-2 mr-sm-2">
          <input
            type="number"
            min="0"
            max="59"
            class="form-control"
            bind:value={toState.mins}
            disabled={betweenDisabled}
          />
          <div class="input-group-append">
            <div class="input-group-text">m</div>
          </div>
        </div>
      </Col>
    </Row>
  </ModalBody>
  <ModalFooter>
    <Button
      color="primary"
      onclick={() => {
        isOpen = false;
        pendingDuration.lessThan = hoursAndMinsToSecs(lessState);
        pendingDuration.moreThan = hoursAndMinsToSecs(moreState);
        pendingDuration.from = hoursAndMinsToSecs(fromState);
        pendingDuration.to = hoursAndMinsToSecs(toState);
        setFilter({duration: pendingDuration});
      }}
    >
      Close & Apply
    </Button>
    <Button
      color="warning"
      onclick={() => {
        resetStates();
      }}>Reset Values</Button
    >
    <Button
      color="danger"
      onclick={() => {
        isOpen = false;
        resetStates();
        resetPending();
        setFilter({duration: pendingDuration});
      }}>Reset Filter</Button
    >
    <Button onclick={() => (isOpen = false)}>Close</Button>
  </ModalFooter>
</Modal>
