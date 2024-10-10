<!--
    @component Main navbar component; handles view display based on user roles

    Properties:
    - `username String`: Empty string if auth. is disabled, otherwise the username as string
    - `authlevel Number`: The current users authentication level
    - `clusters [String]`: List of cluster names
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

  export let username;
  export let authlevel;
  export let clusters;
  export let roles;

  let isOpen = false;
  let screenSize;

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
      requiredRole: roles.admin,
      href: "/monitoring/systems/",
      icon: "hdd-rack",
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
    {
      title: "Analysis",
      requiredRole: roles.support,
      href: "/monitoring/analysis/",
      icon: "graph-up",
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
  <NavbarToggler on:click={() => (isOpen = !isOpen)} />
  <Collapse
    style="justify-content: space-between"
    {isOpen}
    navbar
    expand="md"
    on:update={({ detail }) => (isOpen = detail.isOpen)}
  >
    <Nav navbar>
      {#if screenSize > 1500 || screenSize < 768}
        <NavbarLinks
          {clusters}
          links={views.filter((item) => item.requiredRole <= authlevel)}
        />
      {:else if screenSize > 1300}
        <NavbarLinks
          {clusters}
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
                direction="right"
                links={views.filter(
                  (item) =>
                    item.requiredRole <= authlevel && item.menu == "Info",
                )}
              />
            </DropdownMenu>
          </Dropdown>
        {/if}
      {:else}
        <NavbarLinks
          {clusters}
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
                direction="right"
                links={views.filter(
                  (item) => item.requiredRole <= authlevel && item.menu == 'Info',
                )}
              />
            </DropdownMenu>
          </Dropdown>
        {/if}
      {/if}
    </Nav>
    <NavbarTools {username} {authlevel} {roles} {screenSize} />
  </Collapse>
</Navbar>
