import { mount } from 'svelte';
import {} from './header.entrypoint.js'
import Logs from './Logs.root.svelte'

mount(Logs, {
    target: document.getElementById('svelte-app'),
    props: {
        isAdmin: isAdmin,
    }
})
