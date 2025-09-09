import { mount } from 'svelte';
import {} from './header.entrypoint.js'
import Status from './Status.root.svelte'

mount(Status, {
    target: document.getElementById('svelte-app'),
    props: {
        presetCluster: infos.cluster,
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig]
    ])
})
