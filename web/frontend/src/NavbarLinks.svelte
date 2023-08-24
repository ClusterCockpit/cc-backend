<script>
    import {
        Icon,
        NavLink,
        Dropdown,
        DropdownToggle,
        DropdownMenu,
        DropdownItem,
    } from "sveltestrap";

    export let clusters; // array of names
    export let links; // array of nav links
</script>

{#each links as item}
    {#if !item.perCluster}
        <NavLink href={item.href} active={window.location.pathname == item.href}
            ><Icon name={item.icon} /> {item.title}</NavLink
        >
    {:else}
        <Dropdown nav inNavbar>
            <DropdownToggle nav caret>
                <Icon name={item.icon} />
                {item.title}
            </DropdownToggle>
            <DropdownMenu class="dropdown-menu-lg-end">
                {#each clusters as cluster}
                    <DropdownItem
                        href={item.href + cluster.name}
                        active={window.location.pathname ==
                            item.href + cluster.name}
                    >
                        {cluster.name}
                    </DropdownItem>
                {/each}
            </DropdownMenu>
        </Dropdown>
    {/if}
{/each}
