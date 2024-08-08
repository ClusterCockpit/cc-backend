<!--
    @component Navbar component; renders in app navigation links as received from upstream

    Properties:
    - `clusters [String]`: List of cluster names
    - `links [Object]`: Pre-filtered link objects based on user auth
 -->

<script>
  import {
    Icon,
    NavLink,
    Dropdown,
    DropdownToggle,
    DropdownMenu,
    DropdownItem,
  } from "@sveltestrap/sveltestrap";

  export let clusters;
  export let links;
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
            active={window.location.pathname == item.href + cluster.name}
          >
            {cluster.name}
          </DropdownItem>
        {/each}
      </DropdownMenu>
    </Dropdown>
  {/if}
{/each}
