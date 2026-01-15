<!--
  @component Main navbar component; handles view display based on user roles

  Properties:
  - `username String`: Empty string if auth. is disabled, otherwise the username as string
  - `authlevel Number`: The current users authentication level
  - `clusterNames [String]`: List of cluster names
  - `subclusterMap map[String][]string`: Map of subclusters by cluster names
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

  /* Svelte 5 Props */
  let { 
    username,
    authlevel,
    clusterNames,
    subclusterMap,
    roles
  } = $props();

  /* Const Init */
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
      // svelte-ignore state_referenced_locally
      requiredRole: roles.user,
      // svelte-ignore state_referenced_locally
      href: `/monitoring/user/${username}`,
      icon: "bar-chart-line",
      perCluster: false,
      listOptions: false,
      menu: "none",
    },
    {
      // svelte-ignore state_referenced_locally
      title: jobsTitle.get(authlevel),
      // svelte-ignore state_referenced_locally
      requiredRole: roles.user,
      href: `/monitoring/jobs/`,
      icon: "card-list",
      perCluster: false,
      listOptions: false,
      menu: "Jobs",
    },
    {
      title: "Tags",
      // svelte-ignore state_referenced_locally
      requiredRole: roles.user,
      href: "/monitoring/tags/",
      icon: "tags",
      perCluster: false,
      listOptions: false,
      menu: "Jobs",
    },
    {
      // svelte-ignore state_referenced_locally
      title: usersTitle.get(authlevel),
      // svelte-ignore state_referenced_locally
      requiredRole: roles.manager,
      href: "/monitoring/users/",
      icon: "people",
      perCluster: true,
      listOptions: true,
      menu: "Groups",
    },
    {
      // svelte-ignore state_referenced_locally
      title: projectsTitle.get(authlevel),
      // svelte-ignore state_referenced_locally
      requiredRole: roles.manager,
      href: "/monitoring/projects/",
      icon: "journals",
      perCluster: true,
      listOptions: true,
      menu: "Groups",
    },
    {
      title: "Nodes",
      // svelte-ignore state_referenced_locally
      requiredRole: roles.support,
      href: "/monitoring/systems/",
      icon: "hdd-rack",
      perCluster: true,
      listOptions: true,
      menu: "Info",
    },
    {
      title: "Analysis",
      // svelte-ignore state_referenced_locally
      requiredRole: roles.support,
      href: "/monitoring/analysis/",
      icon: "graph-up",
      perCluster: true,
      listOptions: false,
      menu: "Info",
    },
    {
      title: "Status",
      // svelte-ignore state_referenced_locally
      requiredRole: roles.admin,
      href: "/monitoring/status/",
      icon: "clipboard-data",
      perCluster: true,
      listOptions: true,
      menu: "Info",
    },
  ];

  /* State Init */
  let isOpen = $state(false);
  let screenSize = $state(0);

  /* Derived */
  const showMax = $derived(screenSize >= 1500);
  const showMid = $derived(screenSize < 1500 && screenSize >= 1300);
  const showSml = $derived(screenSize < 1300 && screenSize >= 768);
  const showBrg = $derived(screenSize < 768);
</script>

<svelte:window bind:innerWidth={screenSize} />

<Navbar color="light" light expand="md" fixed="top">
  <NavbarBrand href="/">
    <img alt="ClusterCockpit Logo" src="/img/logo.png" height="25rem" />
  </NavbarBrand>
  <NavbarToggler onclick={() => (isOpen = !isOpen)} />
  <Collapse
    style="justify-content: space-between"
    expand="md"
    {isOpen}
    navbar
  >
    <Nav navbar>
      {#if showMax || showBrg}
        <NavbarLinks
          {clusterNames}
          {subclusterMap}
          links={views.filter((item) => item.requiredRole <= authlevel)}
        />

      {:else if showMid}
        <NavbarLinks
          {clusterNames}
          {subclusterMap}
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
                {clusterNames}
                {subclusterMap}
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
          {clusterNames}
          {subclusterMap}
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
                {clusterNames}
                {subclusterMap}
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
                {clustersNames}
                {subclusterMap}
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
                {clusterNames}
                {subclusterMap}
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