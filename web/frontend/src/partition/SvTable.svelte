<script>
    import { Table } from 'sveltestrap';

    let data = [
        { username: 'user1', linuxMachine: 'machine1', usage: '20%', role: 'admin' },
        { username: 'user2', linuxMachine: 'machine2', usage: '30%', role: 'user' },
        // Add more data as needed
    ];
</script>

<style>
    .bg-light {
        background-color: #f8f9fa; /* This is the color Bootstrap uses for .bg-light */
    }
</style>

<Table striped>
    <thead>
        <tr>
            <th>Username</th>
            <th>Linux-machine</th>
            <th>Usage</th>
            <th>Role</th>
        </tr>
    </thead>
    <tbody>
        {#each data as item (item.username)}
            <tr>
                <td class=""><p class="font-weight-light">{item.username}</p></td>
                <td class="bg-light">{item.linuxMachine}</td>
                <td class="bg-light">{item.usage}</td>
                <td class="bg-light">{item.role}</td>
            </tr>
        {/each}
    </tbody>
</Table>