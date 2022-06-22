import Header from './Header.svelte'

const headerDomTarget = document.getElementById('svelte-header')

if (headerDomTarget != null) {
    new Header({
        target: headerDomTarget,
        props: { ...header },
    })
}
