<script>
    import { Button, Card, CardText, CardTitle } from 'sveltestrap'
    import { createEventDispatcher } from 'svelte'
    import { fade } from 'svelte/transition'

    const dispatch = createEventDispatcher()

    let message = {msg: '', color: '#d63384'}
    let displayMessage = false

    async function handleUrlSubmit() {
        let form = document.querySelector('#create-url-form')
        let formData = new FormData(form)

        try {
            const res = await fetch(form.action, { method: 'POST', body: formData });
            if (res.ok) {
                let text = await res.text()
                popMessage(text, '#048109')
                form.reset()
            } else {
                let text = await res.text()
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
</script>

<Card>
    <!-- default url  -->
    <form id="create-url-form" method="post" action="/api/url/" class="card-body" on:submit|preventDefault={handleUrlSubmit}>
        <CardTitle class="mb-3">FileBrowser Configuration</CardTitle>
        <CardText><p>Current URL :</p></CardText>
        <div class="mb-3">
            <label for="url" class="form-label">URL</label>
            <input type="text" class="form-control" id="url" name="url" required />
        </div>
        <p style="display: flex; align-items: center;">
            <Button type="submit" color="primary">Submit</Button>
            {#if displayMessage}<div style="margin-left: 1.5em;"><b><code style="color: {message.color};" out:fade>{message.msg}</code></b></div>{/if}
        </p>
    </form>
</Card>