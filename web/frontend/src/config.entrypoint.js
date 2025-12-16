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
        clusters: hClusters.map((c) => c.name) // Defined in Header Template
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig],
            ['resampling', resampleConfig]
    ])
})
