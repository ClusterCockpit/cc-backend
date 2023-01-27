<script>
    import { Button } from 'sveltestrap'

    export let user
    let jwt = ""

    function getUserJwt(username) {
        fetch(`/api/jwt/?username=${username}`)
            .then(res => res.text())
            .then(text => {
                jwt = text
                navigator.clipboard.writeText(text).catch(reason => console.error(reason))
            })
    }
</script>

<td>{user.username}</td>
<td>{user.name}</td>
<td>{user.project}</td>
<td>{user.email}</td>
<td><code>{user.roles.join(', ')}</code></td>
<td>
    {#if ! jwt}
        <Button color="success" on:click={getUserJwt(user.username)}>Gen. JWT</Button>
    {:else}
        <textarea rows="3" cols="20">{jwt}</textarea>
    {/if}
</td>
