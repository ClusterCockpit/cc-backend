import {} from './header.entrypoint.js'
import Node from './Node.root.svelte'

new Node({
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
