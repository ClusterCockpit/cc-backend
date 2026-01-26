<!--
  @component Displays node info, serves links to single node page and lists

  Properties:
  - `cluster String`: The nodes' cluster
  - `subCluster String`: The nodes' subCluster
  - `hostname String`: The nodes' hostname
  - `dataHealth [Bool]`: Array of Booleans depicting state of returned data per metric
  - `nodeJobsData [Object]`: Data returned by GQL for jobs runninig on this node [Default: null] 
-->

<script>
  import { 
    Icon,
    Button,
    Row,
    Col,
    Card,
    CardHeader,
    CardBody,
    Input,
    InputGroup,
    InputGroupText,
  } from "@sveltestrap/sveltestrap";
  import { 
    scramble,
    scrambleNames,
  } from "../../generic/utils.js";

  /* Svelte 5 Props */
  let {
    cluster,
    subCluster,
    hostname,
    hoststate,
    dataHealth,
    nodeJobsData = null,
  } = $props();

  /* Const Init */
  // Node State Colors
  const stateColors = {
    allocated: 'success',
    reserved: 'info',
    idle: 'primary',
    mixed: 'warning',
    down: 'danger',
    unknown: 'dark',
    notindb: 'secondary'
  }

  /* Derived */
  // Not at least one returned, selected metric: NodeHealth warning
  const healthWarn = $derived(!dataHealth.includes(true));
  // At least one non-returned selected metric: Metric config error?
  const metricWarn = $derived(dataHealth.includes(false));
  const userList = $derived(nodeJobsData
    ? Array.from(new Set(nodeJobsData.jobs.items.map((j) => scrambleNames ? scramble(j.user) : j.user))).sort((a, b) => a.localeCompare(b))
    : []
  );
  const projectList = $derived(nodeJobsData
    ? Array.from(new Set(nodeJobsData.jobs.items.map((j) => scrambleNames ? scramble(j.project) : j.project))).sort((a, b) => a.localeCompare(b)) 
    : []);

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
    <Row cols={{xs: 1, lg: 2}}>
      <Col class="mb-2 mb-lg-0">
        <InputGroup size="sm">
          {#if healthWarn}
            <InputGroupText class="flex-grow-1 flex-lg-grow-0">
              <Icon name="exclamation-circle" style="padding-right: 0.5rem;"/>
              <span>Jobs</span>
            </InputGroupText>
            <Button class="flex-grow-1" color="danger" disabled>
              No Metrics
            </Button>
          {:else if metricWarn}
            <InputGroupText class="flex-grow-1 flex-lg-grow-0">
              <Icon name="info-circle" style="padding-right: 0.5rem;"/>
              <span>Jobs</span>
            </InputGroupText>
            <Button class="flex-grow-1" color="warning" disabled>
              Missing Metric
            </Button>
          {:else if nodeJobsData.jobs.count == 1 && nodeJobsData.jobs.items[0].shared == "none"}
            <InputGroupText class="flex-grow-1 flex-lg-grow-0">
              <Icon name="circle-fill" style="padding-right: 0.5rem;"/>
              <span>Jobs</span>
            </InputGroupText>
            <Button class="flex-grow-1" color="success" disabled>
              Exclusive
            </Button>
          {:else if nodeJobsData.jobs.count >= 1 && !(nodeJobsData.jobs.items[0].shared == "none")}
            <InputGroupText class="flex-grow-1 flex-lg-grow-0">
              <Icon name="circle-half" style="padding-right: 0.5rem;"/>
              <span>Jobs</span>
            </InputGroupText>
            <Button class="flex-grow-1" color="success" disabled>
              Shared
            </Button>
          <!-- Fallback -->
          {:else if nodeJobsData.jobs.count >= 1}
            <InputGroupText class="flex-grow-1 flex-lg-grow-0">
              <Icon name="circle-fill" style="padding-right: 0.5rem;"/>
              <span>Jobs</span>
            </InputGroupText>
            <Button class="flex-grow-1" color="success" disabled>
              Running
            </Button>
          {:else}
            <InputGroupText class="flex-grow-1 flex-lg-grow-0">
              <Icon name="circle" style="padding-right: 0.5rem;"/>
              <span>Jobs</span>
            </InputGroupText>
            <Button class="flex-grow-1" color="secondary" disabled>
              None
            </Button>
          {/if}
        </InputGroup>
      </Col>
      <Col>
        <InputGroup size="sm">
          <InputGroupText class="flex-grow-1 flex-lg-grow-0">
            State
          </InputGroupText>
          <Button class="flex-grow-1" color={stateColors[hoststate]} disabled>
            {hoststate.charAt(0).toUpperCase() + hoststate.slice(1)}
          </Button>
        </InputGroup>
      </Col>
    </Row>
    <hr class="my-3"/>
    <!-- JOBS -->
    <InputGroup size="sm" class="justify-content-between mb-3">
      <InputGroupText>
        <Icon name="activity"/>
      </InputGroupText>
      <InputGroupText class="justify-content-center" style="width: 4.4rem;">
        Activity
      </InputGroupText>
      <Input class="flex-grow-1" style="background-color: white;" type="text" value="{nodeJobsData?.jobs?.count || 0} Job{(nodeJobsData?.jobs?.count == 1) ? '': 's'}" disabled />
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
      <a title="Show users active on this node" href="/monitoring/users/?cluster={cluster}&state=running&startTime=last30d&node={hostname}" target="_blank" class="btn btn-outline-primary" role="button" aria-disabled="true" >
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
      <a title="Show projects active on this node" href="/monitoring/projects/?cluster={cluster}&state=running&startTime=last30d&node={hostname}" target="_blank" class="btn btn-outline-primary" role="button" aria-disabled="true" >
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
  </CardBody>
</Card>

