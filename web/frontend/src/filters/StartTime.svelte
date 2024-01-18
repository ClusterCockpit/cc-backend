<script>
    import { createEventDispatcher } from 'svelte'
    import { parse, format, sub } from 'date-fns'
    import { Row, Button, Input, Modal, ModalBody, ModalHeader, ModalFooter, FormGroup } from 'sveltestrap'

    const dispatch = createEventDispatcher()

    export let isModified = false
    export let isOpen = false
    export let from = null
    export let to = null

    let pendingFrom, pendingTo

    const now = new Date(Date.now())
    const ago = sub(now, {months: 1})
    const defaultFrom = {date: format(ago, 'yyyy-MM-dd'), time: format(ago, 'HH:mm')}
    const defaultTo   = {date: format(now, 'yyyy-MM-dd'), time: format(now, 'HH:mm')}

    function reset() {
        pendingFrom = from == null ? defaultFrom : fromRFC3339(from)
        pendingTo   = to   == null ? defaultTo   : fromRFC3339(to)
    }

    reset()

    function toRFC3339({ date, time }, secs = '00') {
        const parsedDate = parse(date+' '+time+':'+secs, 'yyyy-MM-dd HH:mm:ss', new Date())
        return parsedDate.toISOString()
    }

    function fromRFC3339(rfc3339) {
        const parsedDate = new Date(rfc3339)
        return { date: format(parsedDate, 'yyyy-MM-dd'), time: format(parsedDate, 'HH:mm') }
    }

    $: isModified = (from != toRFC3339(pendingFrom) || to != toRFC3339(pendingTo, '59'))
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
                to = toRFC3339(pendingTo, '59')
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
