<!--
  @component Admin settings wrapper

  Properties:
  - `ncontent String`: The homepage notice content
  - `clusterNames [String]`: The available clusternames
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

  /* Svelte 5 Props */
  let {
    ncontent,
    clusterNames
  } = $props();

  /* Const Init*/
  const ccconfig = getContext("cc-config");

  /* State Init */
  let users = $state([]);
  let roles = $state([]);

  /* Functions */
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

  /* on Mount */
  onMount(() => initAdmin());
</script>

<Row cols={2} class="p-2 g-2">
  <Col class="mb-1">
    <AddUser {roles} reloadUser={() => getUserList()} />
  </Col>
  <Col class="mb-1">
    <ShowUsers reloadUser={() => getUserList()} bind:users />
  </Col>
  <Col>
    <EditRole {roles} reloadUser={() => getUserList()} />
  </Col>
  <Col>
    <EditProject reloadUser={() => getUserList()} />
  </Col>
  <Options config={ccconfig} {clusterNames}/>
  <NoticeEdit {ncontent}/>
</Row>
