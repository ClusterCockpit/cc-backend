import {} from './header.entrypoint.js'
import Tags from './Tags.root.svelte'

new Tags({
    target: document.getElementById('svelte-app'),
    props: {
        username: username,
        isAdmin: isAdmin,
        tagmap: tagmap,
    },
    context: new Map([
        ['cc-config', clusterCockpitConfig]
    ])
})


