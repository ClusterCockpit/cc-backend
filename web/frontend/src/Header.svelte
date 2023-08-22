<script>
    import { Icon, Button, InputGroup, Input, Collapse,
             Navbar, NavbarBrand, Nav, NavItem, NavLink, NavbarToggler,
             Dropdown, DropdownToggle, DropdownMenu, DropdownItem, InputGroupText } from 'sveltestrap'

    export let username // empty string if auth. is disabled, otherwise the username as string
    export let authlevel // Integer
    export let clusters // array of names
    export let roles // Role Enum-Like

    let isOpen = false

    const userviews = [
        { title: 'My Jobs',     href: `/monitoring/user/${username}`, icon: 'bar-chart-line-fill' },
        { title: `Job Search`,  href: '/monitoring/jobs/',            icon: 'card-list' },
        { title: 'Tags',        href: '/monitoring/tags/',            icon: 'tags' }
    ]

    const managerviews = [
        { title: 'My Jobs',             href: `/monitoring/user/${username}`, icon: 'bar-chart-line-fill' },
        { title: `Managed Jobs`,        href: '/monitoring/jobs/',            icon: 'card-list' },
        { title: `Managed Users`,       href: '/monitoring/users/',           icon: 'people-fill' },
        { title: 'Tags',                href: '/monitoring/tags/',            icon: 'tags' }
    ]

    const supportviews = [
        { title: 'My Jobs',  href: `/monitoring/user/${username}`, icon: 'bar-chart-line-fill' },
        { title: 'Jobs',     href: '/monitoring/jobs/',            icon: 'card-list' },
        { title: 'Users',    href: '/monitoring/users/',           icon: 'people-fill' },
        { title: 'Projects', href: '/monitoring/projects/',        icon: 'folder' },
        { title: 'Tags',     href: '/monitoring/tags/',            icon: 'tags' }
    ]

    const adminviews = [
        { title: 'My Jobs',  href: `/monitoring/user/${username}`, icon: 'bar-chart-line-fill' },
        { title: 'Jobs',     href: '/monitoring/jobs/',            icon: 'card-list' },
        { title: 'Users',    href: '/monitoring/users/',           icon: 'people-fill' },
        { title: 'Projects', href: '/monitoring/projects/',        icon: 'folder' },
        { title: 'Tags',     href: '/monitoring/tags/',            icon: 'tags' }
    ]

    const viewsPerCluster = [
        { title: 'Analysis', requiredRole: roles.support,  href: '/monitoring/analysis/', icon: 'graph-up' },
        { title: 'Systems',  requiredRole: roles.admin,  href: '/monitoring/systems/',  icon: 'cpu' },
        { title: 'Status',   requiredRole: roles.admin,  href: '/monitoring/status/',   icon: 'cpu' },
    ]
</script>

<Navbar color="light" light expand="lg" fixed="top">
    <NavbarBrand href="/">
        <img alt="ClusterCockpit Logo" src="/img/logo.png" height="25rem">
    </NavbarBrand>
    <NavbarToggler on:click={() => (isOpen = !isOpen)} />
    <Collapse {isOpen} navbar expand="lg" on:update={({ detail }) => (isOpen = detail.isOpen)}>
        <Nav pills>
            {#if authlevel == roles.admin}
                {#each adminviews as item}
                    <NavLink href={item.href} active={window.location.pathname == item.href}><Icon name={item.icon}/> {item.title}</NavLink>
                {/each}
            {:else if authlevel == roles.support}
                {#each supportviews as item}
                    <NavLink href={item.href} active={window.location.pathname == item.href}><Icon name={item.icon}/> {item.title}</NavLink>
                {/each}
            {:else if authlevel == roles.manager}
                {#each managerviews as item}
                    <NavLink href={item.href} active={window.location.pathname == item.href}><Icon name={item.icon}/> {item.title}</NavLink>
                {/each}
            {:else} <!-- Compatibility: Handle "user role" or "no role" as identical-->
                {#each userviews as item}
                    <NavLink href={item.href} active={window.location.pathname == item.href}><Icon name={item.icon}/> {item.title}</NavLink>
                {/each}
            {/if}
            {#each viewsPerCluster.filter(item => item.requiredRole <= authlevel) as item}
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
                <InputGroupText style="cursor:help;" title={(authlevel >= roles.support) ? "Example: 'projectId:a100cd', Types are: jobId | jobName | projectId | arrayJobId | username | name" : "Example: 'jobName:myjob', Types are jobId | jobName | projectId | arrayJobId "}><Icon name="info-circle"/></InputGroupText>
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
