import {} from './header.entrypoint.js'
import Job from './Job.root.svelte'

new Job({
    target: document.getElementById('svelte-app'),
    props: {
        dbid: jobInfos.id
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig]
    ])
})
