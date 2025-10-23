import {} from './header.entrypoint.js'
import Config from './Config.root.svelte'

new Config({
    target: document.getElementById('svelte-app'),
    props: {
        isAdmin: isAdmin,
        isSupport: isSupport,
        isApi: isApi,
        username: username,
        ncontent: ncontent,
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig],
            ['resampling', resampleConfig]
    ])
})
