import { mount } from 'svelte';
import {} from './header.entrypoint.js'
import Systems from './Systems.root.svelte'

mount(Systems, {
    target: document.getElementById('svelte-app'),
    props: {
        displayType: displayType,
        cluster: infos.cluster,
        subCluster: infos.subCluster,
        fromPreset: infos.from,
        toPreset: infos.to
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig],
            ['resampling', resampleConfig]
    ])
})
