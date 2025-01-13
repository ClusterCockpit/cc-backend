<!--
    @component Admin settings wrapper
 -->

<script>
  import { Row, Col } from "@sveltestrap/sveltestrap";
  import { onMount, getContext } from "svelte";
  import EditRole from "./admin/EditRole.svelte";
  import EditProject from "./admin/EditProject.svelte";
  import AddUser from "./admin/AddUser.svelte";
  import ShowUsers from "./admin/ShowUsers.svelte";
  import Options from "./admin/Options.svelte";
  import NoticeEdit from "./admin/NoticeEdit.svelte";

  export let ncontent;

  let users = [];
  let roles = [];

  const ccconfig = getContext("cc-config");

  function getUserList() {
    fetch("/config/users/?via-ldap=false&not-just-user=true")
      .then((res) => res.json())
      .then((usersRaw) => {
        users = usersRaw;
      });
  }

  function getValidRoles() {
    fetch("/config/roles/")
      .then((res) => res.json())
      .then((rolesRaw) => {
        roles = rolesRaw;
      });
  }

  function initAdmin() {
    getUserList();
    getValidRoles();
  }

  onMount(() => initAdmin());
</script>

<Row cols={2} class="p-2 g-2">
  <Col class="mb-1">
    <AddUser {roles} on:reload={getUserList} />
  </Col>
  <Col class="mb-1">
    <ShowUsers on:reload={getUserList} bind:users />
  </Col>
  <Col>
    <EditRole {roles} on:reload={getUserList} />
  </Col>
  <Col>
    <EditProject on:reload={getUserList} />
  </Col>
  <Options config={ccconfig}/>
  <NoticeEdit {ncontent}/>
</Row>
