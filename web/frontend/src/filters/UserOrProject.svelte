<script>
    import { InputGroup, Input } from 'sveltestrap'
    import { createEventDispatcher } from 'svelte'

    const dispatch = createEventDispatcher()

    export let user = ''
    export let project = ''
    export let authLevel
    let mode = 'user', term = ''
    const throttle = 500

    function modeChanged() {
        if (mode == 'user') {
            project = term
            term = user
        } else {
            user = term
            term = project
        }
        termChanged(0)
    }

    let timeoutId = null
    function termChanged(sleep = throttle) {
        if (authLevel == 2) {
            project = term

            if (timeoutId != null)
                clearTimeout(timeoutId)

            timeoutId = setTimeout(() => {
                dispatch('update', {
                    project
                })
            }, sleep)
        } else if (authLevel >= 3) {
            if (mode == 'user')
                user = term
            else
                project = term

            if (timeoutId != null)
                clearTimeout(timeoutId)

            timeoutId = setTimeout(() => {
                dispatch('update', {
                    user,
                    project
                })
            }, sleep)
        }
    }
</script>

{#if authLevel == 2}
    <InputGroup>
        <Input
            type="text" bind:value={term} on:change={() => termChanged()} on:keyup={(event) => termChanged(event.key == 'Enter' ? 0 : throttle)} placeholder='filter project...'
        />
    </InputGroup>
{:else if authLevel >= 3}
    <InputGroup>
        <select style="max-width: 175px;" class="form-select"
            bind:value={mode} on:change={modeChanged}>
            <option value={'user'}>Search User</option>
            <option value={'project'}>Search Project</option>
        </select>
        <Input
            type="text" bind:value={term} on:change={() => termChanged()} on:keyup={(event) => termChanged(event.key == 'Enter' ? 0 : throttle)}
            placeholder={mode == 'user' ? 'filter username...' : 'filter project...'} />
    </InputGroup>
{:else}
    Unauthorized
{/if}
