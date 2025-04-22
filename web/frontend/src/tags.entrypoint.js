import {} from './header.entrypoint.js'
import Tags from './Tags.root.svelte'

new Tags({
    target: document.getElementById('svelte-app'),
    props: {
        // authlevel: authlevel,
        tagmap: tagmap,
    }
})


