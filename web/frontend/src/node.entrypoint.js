import { mount } from 'svelte';
import {} from './header.entrypoint.js'
import Node from './Node.root.svelte'

mount(Node, {
    target: document.getElementById('svelte-app'),
    props: {
        cluster: infos.cluster,
        hostname: infos.hostname,
        from: infos.from,
        to: infos.to
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig]
    ])
})
