<script>
    import { createEventDispatcher, getContext } from 'svelte'
    import { Row, Button, Input, Modal, ModalBody, ModalHeader, ModalFooter, FormGroup } from 'sveltestrap'

    const dispatch = createEventDispatcher()

    export let isModified = false
    export let isOpen = false
    export let from = null
    export let to = null

    let pendingFrom, pendingTo

    function reset() {
        pendingFrom = from == null ? { date: '0000-00-00', time: '00:00' } : fromRFC3339(from)
        pendingTo   = to   == null ? { date: '0000-00-00', time: '00:00' } : fromRFC3339(to)
    }

    reset()

    function toRFC3339({ date, time }, secs = 0) {
        const dparts = date.split('-')
        const tparts = time.split(':')
        const d = new Date(
            Number.parseInt(dparts[0]),
            Number.parseInt(dparts[1]) - 1,
            Number.parseInt(dparts[2]),
            Number.parseInt(tparts[0]),
            Number.parseInt(tparts[1]), secs)
        return d.toISOString()
    }

    function fromRFC3339(rfc3339) {
        const d = new Date(rfc3339)
        const pad = (n) => n.toString().padStart(2, '0')
        const date = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
        const time = `${pad(d.getHours())}:${pad(d.getMinutes())}`
        return { date, time }
    }

    $: isModified = (from != toRFC3339(pendingFrom) || to != toRFC3339(pendingTo, 59))
        && !(from == null && pendingFrom.date == '0000-00-00' && pendingFrom.time == '00:00')
        && !(to == null && pendingTo.date == '0000-00-00' && pendingTo.time == '00:00')
</script>

<Modal isOpen={isOpen} toggle={() => (isOpen = !isOpen)}>
    <ModalHeader>
        Select Start Time
    </ModalHeader>
    <ModalBody>
        <h4>From</h4>
        <Row>
            <FormGroup class="col">
                <Input type="date" bind:value={pendingFrom.date}/>
            </FormGroup>
            <FormGroup class="col">
                <Input type="time" bind:value={pendingFrom.time}/>
            </FormGroup>
        </Row>
        <h4>To</h4>
        <Row>
            <FormGroup class="col">
                <Input type="date" bind:value={pendingTo.date}/>
            </FormGroup>
            <FormGroup class="col">
                <Input type="time" bind:value={pendingTo.time}/>
            </FormGroup>
        </Row>
    </ModalBody>
    <ModalFooter>
        <Button color="primary"
            disabled={pendingFrom.date == '0000-00-00' || pendingTo.date == '0000-00-00'}
            on:click={() => {
                isOpen = false
                from = toRFC3339(pendingFrom)
                to = toRFC3339(pendingTo, 59)
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
