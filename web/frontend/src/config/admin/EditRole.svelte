<script>
    import { Card, CardTitle, CardBody } from 'sveltestrap'
    import { createEventDispatcher } from 'svelte'
    import { fade } from 'svelte/transition'

    const dispatch = createEventDispatcher()

    let message = {msg: '', color: '#d63384'}
    let displayMessage = false

    export let roles = []

    async function handleAddRole() {
        const username = document.querySelector('#role-username').value
        const role = document.querySelector('#role-select').value

        if (username == "" || role == "") {
            alert('Please fill in a username and select a role.')
            return
        }

        let formData = new FormData()
        formData.append('username', username)
        formData.append('add-role', role)

        try {
            const res = await fetch(`/api/user/${username}`, { method: 'POST', body: formData })
            if (res.ok) {
                let text = await res.text()
                popMessage(text, '#048109')
                reloadUserList()
            } else {
                let text = await res.text()
                // console.log(res.statusText)
                throw new Error('Response Code ' + res.status + '-> ' + text)
            }
        } catch (err)  {
            popMessage(err, '#d63384')
        }
    }

    async function handleRemoveRole() {
        const username = document.querySelector('#role-username').value
        const role = document.querySelector('#role-select').value

        if (username == "" || role == "") {
            alert('Please fill in a username and select a role.')
            return
        }

        let formData = new FormData()
        formData.append('username', username)
        formData.append('remove-role', role)

        try {
            const res = await fetch(`/api/user/${username}`, { method: 'POST', body: formData })
            if (res.ok) {
                let text = await res.text()
                popMessage(text, '#048109')
                reloadUserList()
            } else {
                let text = await res.text()
                // console.log(res.statusText)
                throw new Error('Response Code ' + res.status + '-> ' + text)
            }
        } catch (err)  {
            popMessage(err, '#d63384')
        }
    }

    function popMessage(response, rescolor) {
        message = {msg: response, color: rescolor}
        displayMessage = true
        setTimeout(function() {
          displayMessage = false
        }, 3500)
    }

    function reloadUserList() {
        dispatch('reload')
    }
</script>

<Card>
    <CardBody>
        <CardTitle class="mb-3">Edit User Roles</CardTitle>
        <div class="input-group mb-3">
            <input type="text" class="form-control" placeholder="username" id="role-username"/>
            <select class="form-select" id="role-select">
                <option selected value="">Role...</option>
                {#each roles as role}
                    <option value={role}>{role.charAt(0).toUpperCase() + role.slice(1)}</option>
                {/each}
            </select>
            <!-- PreventDefault on Sveltestrap-Button more complex to achieve than just use good ol' html button -->
            <!-- see: https://stackoverflow.com/questions/69630422/svelte-how-to-use-event-modifiers-in-my-own-components -->
            <button class="btn btn-primary" type="button" id="add-role-button" on:click|preventDefault={handleAddRole}>Add</button>
            <button class="btn btn-danger" type="button" id="remove-role-button" on:click|preventDefault={handleRemoveRole}>Remove</button>
        </div>
        <p>
            {#if displayMessage}<b><code style="color: {message.color};" out:fade>Update: {message.msg}</code></b>{/if}
        </p>
    </CardBody>
</Card>
