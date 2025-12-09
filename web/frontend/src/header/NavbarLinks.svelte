<!--
  @component Navbar component; renders in app navigation links as received from upstream

  Properties:
  - `clusters [String]`: List of cluster names
  - `subClusters map[String][]string`: Map of subclusters by cluster names
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
    clusters,
    subClusters,
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
          {#each clusters as cluster}
            <Dropdown nav direction="right">
              <DropdownToggle nav caret class="dropdown-item py-1 px-2">
                {cluster.name}
              </DropdownToggle>
              <DropdownMenu>
                <DropdownItem class="py-1 px-2"
                  href={item.href + cluster.name}
                >
                  Node Overview
                </DropdownItem>
                <DropdownItem class="py-1 px-2"
                  href={item.href + 'list/' + cluster.name}
                >
                  Node List
                </DropdownItem>
                {#each subClusters[cluster.name] as subCluster}
                  <DropdownItem class="py-1 px-2"
                    href={item.href + 'list/' + cluster.name + '/' + subCluster}
                  >
                  {subCluster} Node List
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
          {#each clusters as cluster}
            <Dropdown nav direction="right">
              <DropdownToggle nav caret class="dropdown-item py-1 px-2">
                {cluster.name}
              </DropdownToggle>
              <DropdownMenu>
                <DropdownItem class="py-1 px-2"
                  href={item.href + cluster.name}
                >
                  Status Dashboard
                </DropdownItem>
                <DropdownItem class="py-1 px-2"
                  href={item.href + 'detail/' + cluster.name}
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
          {#each clusters as cluster}
            <Dropdown nav direction="right">
              <DropdownToggle nav caret class="dropdown-item py-1 px-2">
                {cluster.name}
              </DropdownToggle>
              <DropdownMenu>
                <DropdownItem class="py-1 px-2"
                  href={item.href + '?cluster=' + cluster.name}
                >
                  All Jobs
                </DropdownItem>
                <DropdownItem class="py-1 px-2"
                  href={item.href + '?cluster=' + cluster.name + '&state=running'}
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
