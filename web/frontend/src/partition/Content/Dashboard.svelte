<!-- Dashboard.svelte -->
<script>
    let data = {
        users: 100,
        revenue: "$10,000",
        newUsers: 20,
    };
</script>

<div class="container">
    <h2>Dashboard</h2>
    <div class="card">
        <div class="card-body">
            <h5 class="card-title">Total Users</h5>
            <p class="card-text">{data.users}</p>
        </div>
    </div>
    <div class="card">
        <div class="card-body">
            <h5 class="card-title">Total Revenue</h5>
            <p class="card-text">{data.revenue}</p>
        </div>
    </div>
    <div class="card">
        <div class="card-body">
            <h5 class="card-title">New Users Today</h5>
            <p class="card-text">{data.newUsers}</p>
        </div>
    </div>
</div>

<style>
    .container {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
        gap: 20px;
    }

    .card {
        border: 1px solid #ccc;
        border-radius: 5px;
        padding: 10px;
        box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
    }
</style>
