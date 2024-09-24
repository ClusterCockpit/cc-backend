import {} from './header.entrypoint.js'
import Job from './Job.root.svelte'

new Job({
    target: document.getElementById('svelte-app'),
    props: {
        dbid: jobInfos.id,
        username: username,
        authlevel: authlevel,
        roles: roles
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig]
    ])
})
