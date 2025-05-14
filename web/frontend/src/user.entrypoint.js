import { mount } from 'svelte';
import {} from './header.entrypoint.js'
import User from './User.root.svelte'

mount(User, {
    target: document.getElementById('svelte-app'),
    props: {
        filterPresets: filterPresets,
        user: userInfos
    },
    context: new Map([
            ['cc-config', clusterCockpitConfig],
            ['resampling', resampleConfig]
    ])
})
