<script>
    import { Table, Input, Button } from "sveltestrap";

    let search = "";
    let data = [
        {
            username: "User1",
            organization: "Org1",
            fullName: "Full Name 1",
            lastActive: "2022-01-01",
        },
        {
            username: "User2",
            organization: "Org2",
            fullName: "Full Name 2",
            lastActive: "2022-01-02",
        },
        {
            username: "User3",
            organization: "Org3",
            fullName: "Full Name 3",
            lastActive: "2022-01-03",
        },
    ];
</script>

<Button color="primary" class="mb-3">New User</Button>
<Input type="search" placeholder="Search..." bind:value={search} class="mb-3" />

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
        {#each data.filter((item) => Object.values(item).some((value) => value
                    .toLowerCase()
                    .includes(search.toLowerCase()))) as item (item.username)}
            <tr>
                <td class=""
                    ><p class="font-weight-light">{item.username}</p></td
                >
                <td class="bg-light">{item.linuxMachine}</td>
                <td class="bg-light">{item.usage}</td>
                <td class="bg-light">{item.role}</td>
            </tr>
        {/each}
    </tbody>
</Table>

<style>
    .bg-light {
        background-color: #f8f9fa; /* This is the color Bootstrap uses for .bg-light */
    }
</style>
