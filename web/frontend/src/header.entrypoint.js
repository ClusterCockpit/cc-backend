import { mount } from 'svelte';
import Header from './Header.svelte';

const headerDomTarget = document.getElementById('svelte-header');

if (headerDomTarget != null) {
    mount(Header, {
        target: headerDomTarget,
        props: {
            username: hUsername, 
            authlevel: hAuthlevel,
            clusters: hClusters, 
            subClusters: hSubClusters,
            roles: hRoles,
        },
    });
}
