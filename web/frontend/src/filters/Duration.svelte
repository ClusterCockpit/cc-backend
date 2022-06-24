<script>
    import { createEventDispatcher } from 'svelte'
    import { Row, Col, Button, Modal, ModalBody, ModalHeader, ModalFooter, FormGroup } from 'sveltestrap'

    const dispatch = createEventDispatcher()

    export let isOpen = false
    export let from = null
    export let to = null

    let pendingFrom, pendingTo

    function reset() {
        pendingFrom = from == null ? { hours: 0, mins: 0 } : secsToHoursAndMins(from)
        pendingTo   = to   == null ? { hours: 0, mins: 0 } : secsToHoursAndMins(to)
    }

    reset()

    function secsToHoursAndMins(duration) {
        const hours = Math.floor(duration / 3600)
        duration -= hours * 3600
        const mins = Math.floor(duration / 60)
        return { hours, mins }
    }

    function hoursAndMinsToSecs({ hours, mins }) {
        return hours * 3600 + mins * 60
    }
</script>

<Modal isOpen={isOpen} toggle={() => (isOpen = !isOpen)}>
    <ModalHeader>
        Select Start Time
    </ModalHeader>
    <ModalBody>
        <h4>Between</h4>
        <Row>
            <Col>
                <div class="input-group mb-2 mr-sm-2">
                    <input type="number" class="form-control"  bind:value={pendingFrom.hours}>
                    <div class="input-group-append">
                        <div class="input-group-text">h</div>
                    </div>
                </div>
            </Col>
            <Col>
                <div class="input-group mb-2 mr-sm-2">
                    <input type="number" class="form-control" bind:value={pendingFrom.mins}>
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
                    <input type="number" class="form-control" bind:value={pendingTo.hours}>
                    <div class="input-group-append">
                        <div class="input-group-text">h</div>
                    </div>
                </div>
            </Col>
            <Col>
                <div class="input-group mb-2 mr-sm-2">
                    <input type="number" class="form-control" bind:value={pendingTo.mins}>
                    <div class="input-group-append">
                        <div class="input-group-text">m</div>
                    </div>
                </div>
            </Col>
        </Row>
    </ModalBody>
    <ModalFooter>
        <Button color="primary"
            on:click={() => {
                isOpen = false
                from = hoursAndMinsToSecs(pendingFrom)
                to = hoursAndMinsToSecs(pendingTo)
                dispatch('update', { from, to })
            }}>
            Close & Apply
        </Button>
        <Button color="danger" on:click={() => {
            isOpen = false
            from = null
            to = null
            reset()
            dispatch('update', { from, to })
        }}>Reset</Button>
        <Button on:click={() => (isOpen = false)}>Close</Button>
    </ModalFooter>
</Modal>
