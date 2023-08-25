<script>
    import {
        Icon,
        Button,
        InputGroup,
        Input,
        Collapse,
        Navbar,
        NavbarBrand,
        Nav,
        NavItem,
        NavbarToggler,
        Dropdown,
        DropdownToggle,
        DropdownMenu,
        InputGroupText,
    } from "sveltestrap";
    import NavbarLinks from "./NavbarLinks.svelte";

    export let username; // empty string if auth. is disabled, otherwise the username as string
    export let authlevel; // Integer
    export let clusters; // array of names
    export let roles; // Role Enum-Like

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

    const views = [
        {
            title: "My Jobs",
            requiredRole: roles.user,
            href: `/monitoring/user/${username}`,
            icon: "bar-chart-line-fill",
            perCluster: false,
            menu: "none",
        },
        {
            title: jobsTitle.get(authlevel),
            requiredRole: roles.user,
            href: `/monitoring/jobs/`,
            icon: "card-list",
            perCluster: false,
            menu: "none",
        },
        {
            title: usersTitle.get(authlevel),
            requiredRole: roles.manager,
            href: "/monitoring/users/",
            icon: "people-fill",
            perCluster: false,
            menu: "Groups",
        },
        {
            title: "Projects",
            requiredRole: roles.support,
            href: "/monitoring/projects/",
            icon: "folder",
            perCluster: false,
            menu: "Groups",
        },
        {
            title: "Tags",
            requiredRole: roles.user,
            href: "/monitoring/tags/",
            icon: "tags",
            perCluster: false,
            menu: "Groups",
        },
        {
            title: "Analysis",
            requiredRole: roles.support,
            href: "/monitoring/analysis/",
            icon: "graph-up",
            perCluster: true,
            menu: "Stats",
        },
        {
            title: "Nodes",
            requiredRole: roles.admin,
            href: "/monitoring/systems/",
            icon: "cpu",
            perCluster: true,
            menu: "Groups",
        },
        {
            title: "Status",
            requiredRole: roles.admin,
            href: "/monitoring/status/",
            icon: "cpu",
            perCluster: true,
            menu: "Stats",
        },
    ];
</script>

<svelte:window bind:innerWidth={screenSize} />
<Navbar color="light" light expand="sm" fixed="top">
    <NavbarBrand href="/">
        <img alt="ClusterCockpit Logo" src="/img/logo.png" height="25rem" />
    </NavbarBrand>
    <NavbarToggler on:click={() => (isOpen = !isOpen)} />
    <Collapse
        {isOpen}
        navbar
        expand="sm"
        on:update={({ detail }) => (isOpen = detail.isOpen)}
    >
        <Nav navbar>
            {#if screenSize > 1500 || screenSize < 576}
                <NavbarLinks
                    {clusters}
                    links={views.filter(
                        (item) => item.requiredRole <= authlevel
                    )}
                />
            {:else if screenSize > 1300}
                <NavbarLinks
                    {clusters}
                    links={views.filter(
                        (item) =>
                            item.requiredRole <= authlevel &&
                            item.menu != "Stats"
                    )}
                />
                <Dropdown nav>
                    <DropdownToggle nav caret>
                        <Icon name="graph-up" />
                        Stats
                    </DropdownToggle>
                    <DropdownMenu class="dropdown-menu-lg-end">
                        <NavbarLinks
                            {clusters}
                            links={views.filter(
                                (item) =>
                                    item.requiredRole <= authlevel &&
                                    item.menu == "Stats"
                            )}
                        />
                    </DropdownMenu>
                </Dropdown>
            {:else}
                <NavbarLinks
                    {clusters}
                    links={views.filter(
                        (item) =>
                            item.requiredRole <= authlevel &&
                            item.menu == "none"
                    )}
                />
                {#each Array("Groups", "Stats") as menu}
                    <Dropdown nav>
                        <DropdownToggle nav caret>
                            {menu}
                        </DropdownToggle>
                        <DropdownMenu class="dropdown-menu-lg-end">
                            <NavbarLinks
                                {clusters}
                                links={views.filter(
                                    (item) =>
                                        item.requiredRole <= authlevel &&
                                        item.menu == menu
                                )}
                            />
                        </DropdownMenu>
                    </Dropdown>
                {/each}
            {/if}
        </Nav>
    </Collapse>
    <Nav>
        <NavItem>
            <form method="GET" action="/search">
                <InputGroup>
                    <Input
                        type="text"
                        placeholder="Search 'type:<query>' ..."
                        name="searchId"
                    />
                    <Button outline type="submit"><Icon name="search" /></Button
                    >
                    <InputGroupText
                        style="cursor:help;"
                        title={authlevel >= roles.support
                            ? "Example: 'projectId:a100cd', Types are: jobId | jobName | projectId | arrayJobId | username | name"
                            : "Example: 'jobName:myjob', Types are jobId | jobName | projectId | arrayJobId "}
                        ><Icon name="info-circle" /></InputGroupText
                    >
                </InputGroup>
            </form>
        </NavItem>
        {#if username}
            <NavItem>
                <form method="POST" action="/logout">
                    <Button
                        outline
                        color="success"
                        type="submit"
                        style="margin-left: 10px;"
                    >
                        {#if screenSize > 1630}
                            <Icon name="box-arrow-right" /> Logout {username}
                        {:else}
                            <Icon name="box-arrow-right" />
                        {/if}
                    </Button>
                </form>
            </NavItem>
        {/if}
        <NavItem>
            <Button
                outline
                on:click={() => (window.location.href = "/config")}
                style="margin-left: 10px;"
            >
                <Icon name="gear" />
            </Button>
        </NavItem>
    </Nav>
</Navbar>
