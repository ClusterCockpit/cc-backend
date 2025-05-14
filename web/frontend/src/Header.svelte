<!--
    @component Main navbar component; handles view display based on user roles

    Properties:
    - `username String`: Empty string if auth. is disabled, otherwise the username as string
    - `authlevel Number`: The current users authentication level
    - `clusters [String]`: List of cluster names
    - `subClusters [String]`: List of subCluster names
    - `roles [Number]`: Enum containing available roles
 -->

<script>
  import {
    Icon,
    Collapse,
    Navbar,
    NavbarBrand,
    Nav,
    NavbarToggler,
    Dropdown,
    DropdownToggle,
    DropdownMenu,
  } from "@sveltestrap/sveltestrap";
  import NavbarLinks from "./header/NavbarLinks.svelte";
  import NavbarTools from "./header/NavbarTools.svelte";

  let { username, authlevel, clusters, subClusters, roles } = $props();

  let isOpen = $state(false);
  let screenSize = $state(0);

  let showMax = $derived(screenSize >= 1500);
  let showMid = $derived(screenSize < 1500 && screenSize >= 1300);
  let showSml = $derived(screenSize < 1300 && screenSize >= 768);
  let showBrg = $derived(screenSize < 768);

  const jobsTitle = new Map();
  jobsTitle.set(2, "Job Search");
  jobsTitle.set(3, "Managed Jobs");
  jobsTitle.set(4, "Jobs");
  jobsTitle.set(5, "Jobs");
  const usersTitle = new Map();
  usersTitle.set(3, "Managed Users");
  usersTitle.set(4, "Users");
  usersTitle.set(5, "Users");
  const projectsTitle = new Map();
  projectsTitle.set(3, "Managed Projects");
  projectsTitle.set(4, "Projects");
  projectsTitle.set(5, "Projects");

  const views = [
    {
      title: "My Jobs",
      requiredRole: roles.user,
      href: `/monitoring/user/${username}`,
      icon: "bar-chart-line",
      perCluster: false,
      listOptions: false,
      menu: "none",
    },
    {
      title: jobsTitle.get(authlevel),
      requiredRole: roles.user,
      href: `/monitoring/jobs/`,
      icon: "card-list",
      perCluster: false,
      listOptions: false,
      menu: "Jobs",
    },
    {
      title: "Tags",
      requiredRole: roles.user,
      href: "/monitoring/tags/",
      icon: "tags",
      perCluster: false,
      listOptions: false,
      menu: "Jobs",
    },
    {
      title: usersTitle.get(authlevel),
      requiredRole: roles.manager,
      href: "/monitoring/users/",
      icon: "people",
      perCluster: true,
      listOptions: true,
      menu: "Groups",
    },
    {
      title: projectsTitle.get(authlevel),
      requiredRole: roles.manager,
      href: "/monitoring/projects/",
      icon: "journals",
      perCluster: true,
      listOptions: true,
      menu: "Groups",
    },
    {
      title: "Nodes",
      requiredRole: roles.support,
      href: "/monitoring/systems/",
      icon: "hdd-rack",
      perCluster: true,
      listOptions: true,
      menu: "Info",
    },
    {
      title: "Analysis",
      requiredRole: roles.support,
      href: "/monitoring/analysis/",
      icon: "graph-up",
      perCluster: true,
      listOptions: false,
      menu: "Info",
    },
    {
      title: "Status",
      requiredRole: roles.admin,
      href: "/monitoring/status/",
      icon: "clipboard-data",
      perCluster: true,
      listOptions: false,
      menu: "Info",
    },
  ];
</script>

<svelte:window bind:innerWidth={screenSize} />

<Navbar color="light" light expand="md" fixed="top">
  <NavbarBrand href="/">
    <img alt="ClusterCockpit Logo" src="/img/logo.png" height="25rem" />
  </NavbarBrand>
  <NavbarToggler onclick={() => (isOpen = !isOpen)} />
  <Collapse
    style="justify-content: space-between"
    {isOpen}
    navbar
    expand="md"
    onupdate={({ detail }) => (isOpen = detail.isOpen)}
  >
    <Nav navbar>
      {#if showMax || showBrg}
        <NavbarLinks
          {clusters}
          {subClusters}
          links={views.filter((item) => item.requiredRole <= authlevel)}
        />

      {:else if showMid}
        <NavbarLinks
          {clusters}
          {subClusters}
          links={views.filter(
            (item) => item.requiredRole <= authlevel && item.menu != "Info",
          )}
        />
        {#if authlevel >= 4} <!-- Support+ Info Menu-->
          <Dropdown nav>
            <DropdownToggle nav caret>
              <Icon name="graph-up" />
              Info
            </DropdownToggle>
            <DropdownMenu class="dropdown-menu-lg-end">
              <NavbarLinks
                {clusters}
                {subClusters}
                direction="right"
                links={views.filter(
                  (item) =>
                    item.requiredRole <= authlevel && item.menu == "Info",
                )}
              />
            </DropdownMenu>
          </Dropdown>
        {/if}

      {:else if showSml}
        <NavbarLinks
          {clusters}
          {subClusters}
          links={views.filter(
            (item) => item.requiredRole <= authlevel && item.menu == "none",
          )}
        />
        {#if authlevel >= 2} <!-- User+ Job Menu -->
          <Dropdown nav>
            <DropdownToggle nav caret>
              Jobs
            </DropdownToggle>
            <DropdownMenu class="dropdown-menu-lg-end">
              <NavbarLinks
                {clusters}
                {subClusters}
                direction="right"
                links={views.filter(
                  (item) => item.requiredRole <= authlevel && item.menu == 'Jobs',
                )}
              />
            </DropdownMenu>
          </Dropdown>
        {/if}
        {#if authlevel >= 3} <!-- Manager+ Group Lists Menu-->
          <Dropdown nav>
            <DropdownToggle nav caret>
              Groups
            </DropdownToggle>
            <DropdownMenu class="dropdown-menu-lg-end">
              <NavbarLinks
                {clusters}
                {subClusters}
                direction="right"
                links={views.filter(
                  (item) => item.requiredRole <= authlevel && item.menu == 'Groups',
                )}
              />
            </DropdownMenu>
          </Dropdown>
        {/if}
        {#if authlevel >= 4} <!-- Support+ Info Menu-->
          <Dropdown nav>
            <DropdownToggle nav caret>
              Info
            </DropdownToggle>
            <DropdownMenu class="dropdown-menu-lg-end">
              <NavbarLinks
                {clusters}
                {subClusters}
                direction="right"
                links={views.filter(
                  (item) => item.requiredRole <= authlevel && item.menu == 'Info',
                )}
              />
            </DropdownMenu>
          </Dropdown>
        {/if}

      {:else}
          <span>Error: Unknown Window Size!</span>
      {/if}
    </Nav>
    <NavbarTools {username} {authlevel} {roles} {screenSize} />
  </Collapse>
</Navbar>