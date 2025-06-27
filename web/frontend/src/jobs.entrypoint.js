import { mount } from 'svelte';
import {} from './header.entrypoint.js'
import Jobs from './Jobs.root.svelte'

mount(Jobs, {
    target: document.getElementById('svelte-app'),
    props: {
        filterPresets: filterPresets,
        authlevel: authlevel,
        roles: roles
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig],
            ['resampling', resampleConfig]
    ])
})
