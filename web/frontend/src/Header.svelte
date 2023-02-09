<script>
    import { Icon, Button, InputGroup, Input, Collapse,
             Navbar, NavbarBrand, Nav, NavItem, NavLink, NavbarToggler,
             Dropdown, DropdownToggle, DropdownMenu, DropdownItem, InputGroupText } from 'sveltestrap'

    export let username // empty string if auth. is disabled, otherwise the username as string
    export let isAdmin  // boolean
    export let clusters // array of names

    let isOpen = false

    const views = [
        isAdmin
            ? { title: 'Jobs',     adminOnly: false, href: '/monitoring/jobs/',            icon: 'card-list' }
            : { title: 'My Jobs',  adminOnly: false, href: `/monitoring/user/${username}`, icon: 'bar-chart-line-fill' },
        { title: 'Users',    adminOnly: true,  href: '/monitoring/users/',           icon: 'people-fill' },
        { title: 'Projects', adminOnly: true,  href: '/monitoring/projects/',        icon: 'folder' },
        { title: 'Tags',     adminOnly: false, href: '/monitoring/tags/',            icon: 'tags' }
    ]
    const viewsPerCluster = [
        { title: 'Analysis', adminOnly: true,  href: '/monitoring/analysis/', icon: 'graph-up' },
        { title: 'Systems',  adminOnly: true,  href: '/monitoring/systems/',  icon: 'cpu' },
        { title: 'Status',   adminOnly: true,  href: '/monitoring/status/',   icon: 'cpu' },
    ]
</script>

<Navbar color="light" light expand="lg" fixed="top">
    <NavbarBrand href="/">
        <img alt="ClusterCockpit Logo" src="/img/logo.png" height="25rem">
    </NavbarBrand>
    <NavbarToggler on:click={() => (isOpen = !isOpen)} />
    <Collapse {isOpen} navbar expand="lg" on:update={({ detail }) => (isOpen = detail.isOpen)}>
        <Nav pills>
            {#each views.filter(item => isAdmin || !item.adminOnly) as item}
                <NavLink href={item.href} active={window.location.pathname == item.href}><Icon name={item.icon}/> {item.title}</NavLink>
            {/each}
            {#each viewsPerCluster.filter(item => !item.adminOnly || isAdmin) as item}
                <NavItem>
                    <Dropdown nav inNavbar>
                        <DropdownToggle nav caret>
                            <Icon name={item.icon}/> {item.title}
                        </DropdownToggle>
                        <DropdownMenu>
                            {#each clusters as cluster}
                                <DropdownItem href={item.href + cluster.name} active={window.location.pathname == item.href + cluster.name}>
                                    {cluster.name}
                                </DropdownItem>
                            {/each}
                        </DropdownMenu>
                    </Dropdown>
                </NavItem>
            {/each}
        </Nav>
    </Collapse>
    <div class="d-flex">
        <form method="GET" action="/search">
            <InputGroup>
                <Input type="text" placeholder="Search 'type:<query>' ..." name="searchId"/>
                <Button outline type="submit"><Icon name="search"/></Button>
                <InputGroupText style="cursor:help;" title={isAdmin ? "Example: 'projectId:a100cd', Types are: jobId | jobName | projectId | username" : "Example: 'jobName:myjob', Types are jobId | jobName"}><Icon name="info-circle"/></InputGroupText>
            </InputGroup>
        </form>
        {#if username}
            <form method="POST" action="/logout">
                <Button outline color="success" type="submit" style="margin-left: 10px;">
                    <Icon name="box-arrow-right"/> Logout {username}
                </Button>
            </form>
        {/if}
        <Button outline on:click={() => window.location.href = '/config'} style="margin-left: 10px;">
            <Icon name="gear"/>
        </Button>
    </div>
</Navbar>
