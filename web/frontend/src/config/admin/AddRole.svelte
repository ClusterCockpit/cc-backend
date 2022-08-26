<script>
    import { Card, CardTitle, CardBody } from 'sveltestrap'
    import { createEventDispatcher } from 'svelte'
    import { fade } from 'svelte/transition'

    const dispatch = createEventDispatcher()

    let message = {msg: '', color: '#d63384'}
    let displayMessage = false

    async function handleAddRole() {
        const username = document.querySelector('#add-role-username').value
        const role = document.querySelector('#add-role-select').value

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
                throw new Error('Response Code ' + res.status + '-> ' + res.statusText)
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
        <CardTitle class="mb-3">Add Role to User</CardTitle>
        <div class="input-group mb-3">
            <input type="text" class="form-control" placeholder="username" id="add-role-username"/>
            <select class="form-select" id="add-role-select">
                <option selected value="">Role...</option>
                <option value="user">User</option>
                <option value="admin">Admin</option>
                <option value="api">API</option>
            </select>
            <!-- PreventDefault on Sveltestrap-Button more complex to achieve than just use good ol' html button -->
            <!-- see: https://stackoverflow.com/questions/69630422/svelte-how-to-use-event-modifiers-in-my-own-components -->
            <button class="btn btn-primary" type="button" id="add-role-button" on:click|preventDefault={handleAddRole}>Add</button>
        </div>
        <p>
            {#if displayMessage}<b><code style="color: {message.color};" out:fade>Update: {message.msg}</code></b>{/if}
        </p>
    </CardBody>
</Card>
