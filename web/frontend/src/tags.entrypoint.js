import { mount } from 'svelte';
import {} from './header.entrypoint.js'
import Tags from './Tags.root.svelte'

mount(Tags, {
    target: document.getElementById('svelte-app'),
    props: {
        username: username,
        isAdmin: isAdmin,
        presetTagmap: tagmap,
    },
    context: new Map([
        ['cc-config', clusterCockpitConfig]
    ])
})
