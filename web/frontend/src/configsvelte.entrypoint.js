import {} from './header.entrypoint.js'
import Configsvelte from './Configsvelte.root.svelte'

new Configsvelte({
    target: document.getElementById('svelte-app'),
    props: {
        user: user
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig]
    ])
})
