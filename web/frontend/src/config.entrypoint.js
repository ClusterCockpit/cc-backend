import {} from './header.entrypoint.js'
import Config from './Config.root.svelte'

new Config({
    target: document.getElementById('svelte-app'),
    props: {
        isAdmin: isAdmin,
        isApi: isApi,
        username: username
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig]
    ])
})
