<script>
    import SideNav from "./SideNav.svelte";
    import Dashboard from "./Content/Dashboard.svelte";
    let activeContent = "Dashboard";

    function handleContentChange(event) {
        activeContent = event.detail;
    }
</script>

<SideNav on:change={handleContentChange} />

<div>
    {#if activeContent === "Dashboard"}
        <Dashboard />
    {:else if activeContent === "Create LV"}
        <!-- <CreateLV /> -->
        <Dashboard />

    {:else if activeContent === "Monitor"}
        <!-- <Monitor /> -->
        <Dashboard />

        {:else if activeContent === "Auto Storage"}
        <!-- <AutoStorage /> -->
        <Dashboard />

        {:else if activeContent === "Settings"}
        <!-- <Settings /> -->
        <Dashboard />
    {/if}
</div>
