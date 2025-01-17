<!--
    @component Displays node info, serves links to single node page and lists

    Properties:
    - `cluster String`: The nodes' cluster
    - `subCluster String`: The nodes' subCluster
    - `cluster String`: The nodes' hostname
 -->

<script>
  import { 
    Spinner,
    Icon,
    Button,
    Card,
    CardHeader,
    CardBody,
    Input,
    InputGroup,
    InputGroupText, } from "@sveltestrap/sveltestrap";
  import {
    queryStore,
    gql,
    getContextClient,
  } from "@urql/svelte";

  export let cluster;
  export let subCluster
  export let hostname;
  export let dataHealth;

  const client = getContextClient();
  const paging = { itemsPerPage: 50, page: 1 };
  const sorting = { field: "startTime", type: "col", order: "DESC" };
  const filter = [
    { cluster: { eq: cluster } },
    { node: { contains: hostname } },
    { state: ["running"] },
  ];

  const nodeJobsQuery = gql`
    query (
      $filter: [JobFilter!]!
      $sorting: OrderByInput!
      $paging: PageRequest!
    ) {
      jobs(filter: $filter, order: $sorting, page: $paging) {
        items {
          user
          project
          exclusive
        }
        count
      }
    }
  `;

  // Not at least one returned, selected metric: NodeHealth warning
  const healthWarn = !dataHealth.includes(true);
  // At least one non-returned selected metric: Metric config error?
  const metricWarn = dataHealth.includes(false);

  $: nodeJobsData = queryStore({
    client: client,
    query: nodeJobsQuery,
    variables: { paging, sorting, filter },
  });

  let userList;
  let projectList;
   $: if ($nodeJobsData?.data) {
    userList = Array.from(new Set($nodeJobsData.data.jobs.items.map((j) => j.user))).sort((a, b) => a.localeCompare(b));
    projectList = Array.from(new Set($nodeJobsData.data.jobs.items.map((j) => j.project))).sort((a, b) => a.localeCompare(b));
   }

</script>

<Card class="pb-3">
  <CardHeader class="d-inline-flex justify-content-between align-items-end">
    <div>
      <h5 class="mb-0">
        Node
        <a href="/monitoring/node/{cluster}/{hostname}" target="_blank">
          {hostname}
        </a>
      </h5>
    </div>
    <div class="text-capitalize">
      <h6 class="mb-0">
        {cluster} {subCluster}
      </h6>
    </div>
  </CardHeader>
  <CardBody>
    {#if $nodeJobsData.fetching}
      <Spinner />
    {:else if $nodeJobsData.data}
      {#if healthWarn}
        <InputGroup>
          <InputGroupText>
            <Icon name="exclamation-circle"/>
          </InputGroupText>
          <InputGroupText>
            Status
          </InputGroupText>
          <Button color="danger" disabled>
            Unhealthy
          </Button>
        </InputGroup>
      {:else if metricWarn}
        <InputGroup>
          <InputGroupText>
            <Icon name="info-circle"/>
          </InputGroupText>
          <InputGroupText>
            Status
          </InputGroupText>
          <Button color="warning" disabled>
            Missing Metric
          </Button>
        </InputGroup>
      {:else if $nodeJobsData.data.jobs.count == 1 && $nodeJobsData.data.jobs.items[0].exclusive}
        <InputGroup>
          <InputGroupText>
            <Icon name="circle-fill"/>
          </InputGroupText>
          <InputGroupText>
            Status
          </InputGroupText>
          <Button color="success" disabled>
            Exclusive
          </Button>
        </InputGroup>
      {:else if $nodeJobsData.data.jobs.count >= 1 && !$nodeJobsData.data.jobs.items[0].exclusive}
        <InputGroup>
          <InputGroupText>
            <Icon name="circle-half"/>
          </InputGroupText>
          <InputGroupText>
            Status
          </InputGroupText>
          <Button color="success" disabled>
            Shared
          </Button>
        </InputGroup>
      {:else}
        <InputGroup>
          <InputGroupText>
            <Icon name="circle"/>
          </InputGroupText>
          <InputGroupText>
            Status
          </InputGroupText>
          <Button color="secondary" disabled>
            Idle
          </Button>
        </InputGroup>
      {/if}
      <hr class="my-3"/>
      <!-- JOBS -->
      <InputGroup size="sm" class="justify-content-between mb-3">
        <InputGroupText>
          <Icon name="activity"/>
        </InputGroupText>
        <InputGroupText class="justify-content-center" style="width: 4.4rem;">
          Activity
        </InputGroupText>
        <Input class="flex-grow-1" style="background-color: white;" type="text" value="{$nodeJobsData?.data?.jobs?.count || 0} Job{($nodeJobsData?.data?.jobs?.count == 1) ? '': 's'}" disabled />
        <a title="Show jobs running on this node" href="/monitoring/jobs/?cluster={cluster}&state=running&node={hostname}" target="_blank" class="btn btn-outline-primary" role="button" aria-disabled="true" >
          <Icon name="view-list" /> 
          List
        </a>
      </InputGroup>
      <!-- USERS -->
      <InputGroup size="sm" class="justify-content-between {(userList?.length > 0) ? 'mb-1' : 'mb-3'}">
        <InputGroupText>
          <Icon name="people"/>
        </InputGroupText>
        <InputGroupText class="justify-content-center" style="width: 4.4rem;">
          Users
        </InputGroupText>
        <Input class="flex-grow-1" style="background-color: white;" type="text" value="{userList?.length || 0} User{(userList?.length == 1) ? '': 's'}" disabled />
        <a title="Show users active on this node" href="/monitoring/users/?cluster={cluster}&state=running&node={hostname}" target="_blank" class="btn btn-outline-primary" role="button" aria-disabled="true" >
          <Icon name="view-list" /> 
          List
        </a>
      </InputGroup>
      {#if userList?.length > 0}
        <Card class="mb-3">
          <div class="p-1">
            {userList.join(", ")}
          </div>
        </Card>
      {/if}
      <!-- PROJECTS -->
      <InputGroup size="sm" class="justify-content-between {(projectList?.length > 0) ? 'mb-1' : 'mb-3'}">
        <InputGroupText>
          <Icon name="journals"/>
        </InputGroupText>
        <InputGroupText class="justify-content-center" style="width: 4.4rem;">
          Projects
        </InputGroupText>
        <Input class="flex-grow-1" style="background-color: white;" type="text" value="{projectList?.length || 0} Project{(projectList?.length == 1) ? '': 's'}" disabled />
        <a title="Show projects active on this node" href="/monitoring/projects/?cluster={cluster}&state=running&node={hostname}" target="_blank" class="btn btn-outline-primary" role="button" aria-disabled="true" >
          <Icon name="view-list" /> 
          List
        </a>
      </InputGroup>
      {#if projectList?.length > 0}
        <Card>
          <div class="p-1">
            {projectList.join(", ")}
          </div>
        </Card>
      {/if}
    {/if}
  </CardBody>
</Card>

