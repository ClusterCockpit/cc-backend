<script>
    import { createEventDispatcher } from 'svelte'
    import { Row, Col, Button, Modal, ModalBody, ModalHeader, ModalFooter } from 'sveltestrap'

    const dispatch = createEventDispatcher()

    export let isOpen   = false
    export let lessThan = null
    export let moreThan = null
    export let from     = null
    export let to       = null

    let pendingLessThan, pendingMoreThan, pendingFrom, pendingTo
    let lessDisabled = false, moreDisabled = false, betweenDisabled = false

    function reset() {
        pendingLessThan = lessThan == null ? { hours: 0, mins: 0 } : secsToHoursAndMins(lessThan)
        pendingMoreThan = moreThan == null ? { hours: 0, mins: 0 } : secsToHoursAndMins(moreThan)
        pendingFrom     = from     == null ? { hours: 0, mins: 0 } : secsToHoursAndMins(from)
        pendingTo       = to       == null ? { hours: 0, mins: 0 } : secsToHoursAndMins(to)
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

    $: lessDisabled    = pendingMoreThan.hours !== 0 || pendingMoreThan.mins !== 0 || pendingFrom.hours !== 0 || pendingFrom.mins !== 0 || pendingTo.hours !== 0 || pendingTo.mins !== 0
    $: moreDisabled    = pendingLessThan.hours !== 0 || pendingLessThan.mins !== 0 || pendingFrom.hours !== 0 || pendingFrom.mins !== 0 || pendingTo.hours !== 0 || pendingTo.mins !== 0
    $: betweenDisabled = pendingMoreThan.hours !== 0 || pendingMoreThan.mins !== 0 || pendingLessThan.hours !== 0 || pendingLessThan.mins !== 0

</script>

<Modal isOpen={isOpen} toggle={() => (isOpen = !isOpen)}>
    <ModalHeader>
        Select Job Duration
    </ModalHeader>
    <ModalBody>
        <h4>Duration more than</h4>
        <Row>
            <Col>
                <div class="input-group mb-2 mr-sm-2">
                    <input type="number" min="0" class="form-control" bind:value={pendingMoreThan.hours} disabled={moreDisabled}>
                    <div class="input-group-append">
                        <div class="input-group-text">h</div>
                    </div>
                </div>
            </Col>
            <Col>
                <div class="input-group mb-2 mr-sm-2">
                    <input type="number" min="0" max="59" class="form-control" bind:value={pendingMoreThan.mins} disabled={moreDisabled}>
                    <div class="input-group-append">
                        <div class="input-group-text">m</div>
                    </div>
                </div>
            </Col>
        </Row>
        <hr/>

        <h4>Duration less than</h4>
        <Row>
            <Col>
                <div class="input-group mb-2 mr-sm-2">
                    <input type="number" min="0" class="form-control" bind:value={pendingLessThan.hours} disabled={lessDisabled}>
                    <div class="input-group-append">
                        <div class="input-group-text">h</div>
                    </div>
                </div>
            </Col>
            <Col>
                <div class="input-group mb-2 mr-sm-2">
                    <input type="number" min="0" max="59" class="form-control" bind:value={pendingLessThan.mins} disabled={lessDisabled}>
                    <div class="input-group-append">
                        <div class="input-group-text">m</div>
                    </div>
                </div>
            </Col>
        </Row>
        <hr/>

        <h4>Duration between</h4>
        <Row>
            <Col>
                <div class="input-group mb-2 mr-sm-2">
                    <input type="number" min="0" class="form-control" bind:value={pendingFrom.hours} disabled={betweenDisabled}>
                    <div class="input-group-append">
                        <div class="input-group-text">h</div>
                    </div>
                </div>
            </Col>
            <Col>
                <div class="input-group mb-2 mr-sm-2">
                    <input type="number" min="0" max="59" class="form-control" bind:value={pendingFrom.mins} disabled={betweenDisabled}>
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
                    <input type="number" min="0" class="form-control" bind:value={pendingTo.hours} disabled={betweenDisabled}>
                    <div class="input-group-append">
                        <div class="input-group-text">h</div>
                    </div>
                </div>
            </Col>
            <Col>
                <div class="input-group mb-2 mr-sm-2">
                    <input type="number" min="0" max="59" class="form-control" bind:value={pendingTo.mins} disabled={betweenDisabled}>
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
                lessThan = hoursAndMinsToSecs(pendingLessThan)
                moreThan = hoursAndMinsToSecs(pendingMoreThan)
                from = hoursAndMinsToSecs(pendingFrom)
                to = hoursAndMinsToSecs(pendingTo)
                dispatch('update', { lessThan, moreThan, from, to })
            }}>
            Close & Apply
        </Button>
        <Button color="warning" on:click={() => {
            lessThan = null
            moreThan = null
            from = null
            to = null
            reset()
        }}>Reset Values</Button>
        <Button color="danger" on:click={() => {
            isOpen = false
            lessThan = null
            moreThan = null
            from = null
            to = null
            reset()
            dispatch('update', { lessThan, moreThan, from, to })
        }}>Reset Filter</Button>
        <Button on:click={() => (isOpen = false)}>Close</Button>
    </ModalFooter>
</Modal>
