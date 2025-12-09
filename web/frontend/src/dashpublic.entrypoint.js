import { mount } from 'svelte';
// import {} from './header.entrypoint.js'
import DashPublic from './DashPublic.root.svelte'

mount(DashPublic, {
    target: document.getElementById('svelte-app'),
    props: {
        presetCluster: infos.cluster,
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig]
    ])
})
