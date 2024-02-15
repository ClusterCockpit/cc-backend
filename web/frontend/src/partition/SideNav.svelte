<script>
    import { onMount } from "svelte";
    import { createEventDispatcher } from "svelte";
    import { Nav, NavItem } from "sveltestrap";

    let activeItem = "Dashboard";
    const dispatch = createEventDispatcher();

    function handleItemClick(item) {
        activeItem = item;
        dispatch("change", activeItem);
    }

    onMount(() => {
        dispatch("change", activeItem);
    });
</script>

<div class="d-flex flex-column p-3">
    <Nav vertical>
        <NavItem on:click={() => handleItemClick("Dashboard")}>
            <a href="google.com">Dashboard</a>
        </NavItem>
        <NavItem on:click={() => handleItemClick("Create LV")}>
            <a href="google.com">Create LV</a>
        </NavItem>
        <NavItem on:click={() => handleItemClick("Monitor")}>
            <a href="google.com">Monitor</a>
        </NavItem>
        <NavItem on:click={() => handleItemClick("Auto Storage")}>
            <a href="google.com">Auto Storage</a>
        </NavItem>
        <NavItem on:click={() => handleItemClick("Settings")}>
            <a href="google.com">Settings</a>
        </NavItem>
    </Nav>
</div>

{#if activeItem === "Dashboard"}
    <div>Dashboard Content</div>
{:else if activeItem === "Create LV"}
    <div>Create LV Content</div>
{:else if activeItem === "Monitor"}
    <div>Monitor Content</div>
{:else if activeItem === "Auto Storage"}
    <div>Auto Storage Content</div>
{:else if activeItem === "Settings"}
    <div>Settings Content</div>
{/if}
