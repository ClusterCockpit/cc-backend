import {} from './header.entrypoint.js'
import Home from './Home.root.svelte'

new Home({
    target: document.getElementById('svelte-app'),
    props: {
        isAdmin: isAdmin
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig]
    ])
})
