import { mount } from 'svelte';
import {} from './header.entrypoint.js'
import Config from './Config.root.svelte'

mount(Config, {
    target: document.getElementById('svelte-app'),
    props: {
        isAdmin: isAdmin,
        isSupport: isSupport,
        isApi: isApi,
        username: username,
        ncontent: ncontent,
        clusterNames: hClusterNames
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig],
            ['resampling', resampleConfig]
    ])
})
