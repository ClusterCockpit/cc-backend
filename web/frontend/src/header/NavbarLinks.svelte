<!--
  @component Navbar component; renders in app navigation links as received from upstream

  Properties:
  - `clusterNames [String]`: List of cluster names
  - `subclusterMap map[String][]string`: Map of subclusters by cluster names
  - `links [Object]`: Pre-filtered link objects based on user auth
  - `direction String?`: The direcion of the drop-down menue [default: down]
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

  /* Svelte 5 Props */
  let {
    clusterNames,
    subclusterMap,
    links,
    direction = "down"
  } = $props();
</script>

{#each links as item}
  {#if item.listOptions}
    {#if item.title === 'Nodes'}
      <Dropdown nav inNavbar {direction}>
        <DropdownToggle nav caret>
          <Icon name={item.icon} />
          {item.title}
        </DropdownToggle>
        <DropdownMenu class="dropdown-menu-lg-end">
          {#each clusterNames as cn}
            <Dropdown nav direction="right">
              <DropdownToggle nav caret class="dropdown-item py-1 px-2">
                {cn}
              </DropdownToggle>
              <DropdownMenu>
                <DropdownItem class="py-1 px-2"
                  href={item.href + cn}
                >
                  Node Overview
                </DropdownItem>
                <DropdownItem class="py-1 px-2"
                  href={`${item.href}list/${cn}`}
                >
                  Node List
                </DropdownItem>
                {#each subclusterMap[cn] as scn}
                  <DropdownItem class="py-1 px-2"
                    href={`${item.href}list/${cn}/${scn}`}
                  >
                  {scn} Node List
                  </DropdownItem>
                {/each}
              </DropdownMenu>
            </Dropdown>
          {/each}
        </DropdownMenu>
      </Dropdown>
    {:else if item.title === 'Status'}
      <Dropdown nav inNavbar {direction}>
        <DropdownToggle nav caret>
          <Icon name={item.icon} />
          {item.title}
        </DropdownToggle>
        <DropdownMenu class="dropdown-menu-lg-end">
          {#each clusterNames as cn}
            <Dropdown nav direction="right">
              <DropdownToggle nav caret class="dropdown-item py-1 px-2">
                {cn}
              </DropdownToggle>
              <DropdownMenu>
                <DropdownItem class="py-1 px-2"
                  href={item.href + cn}
                >
                  Status Dashboard
                </DropdownItem>
                <DropdownItem class="py-1 px-2"
                  href={`${item.href}detail/${cn}`}
                >
                  Status Details
                </DropdownItem>
              </DropdownMenu>
            </Dropdown>
          {/each}
        </DropdownMenu>
      </Dropdown>
    {:else}
      <Dropdown nav inNavbar {direction}>
        <DropdownToggle nav caret>
          <Icon name={item.icon} />
          {item.title}
        </DropdownToggle>
        <DropdownMenu class="dropdown-menu-lg-end">
          <DropdownItem
            href={item.href}
          >
            All Clusters
          </DropdownItem>
          <DropdownItem divider />
          {#each clusterNames as cn}
            <Dropdown nav direction="right">
              <DropdownToggle nav caret class="dropdown-item py-1 px-2">
                {cn}
              </DropdownToggle>
              <DropdownMenu>
                <DropdownItem class="py-1 px-2"
                  href={`${item.href}?cluster=${cn}`}
                >
                  All Jobs
                </DropdownItem>
                <DropdownItem class="py-1 px-2"
                  href={`${item.href}?cluster=${cn}&state=running`}
                >
                  Running Jobs
                </DropdownItem>
              </DropdownMenu>
            </Dropdown>
          {/each}
        </DropdownMenu>
      </Dropdown>
    {/if}
  {:else if !item.perCluster}
    <NavLink href={item.href} active={window.location.pathname == item.href}
      ><Icon name={item.icon} /> {item.title}</NavLink
    >
  {:else}
    <Dropdown nav inNavbar {direction}>
      <DropdownToggle nav caret>
        <Icon name={item.icon} />
        {item.title}
      </DropdownToggle>
      <DropdownMenu class="dropdown-menu-lg-end">
        {#each clusterNames as cn}
          <DropdownItem
            href={item.href + cn}
            active={window.location.pathname == item.href + cn}
          >
            {cn}
          </DropdownItem>
        {/each}
      </DropdownMenu>
    </Dropdown>
  {/if}
{/each}
