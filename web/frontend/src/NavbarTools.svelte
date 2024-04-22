<script>
  import {
    Icon,
    Nav,
    NavItem,
    InputGroup,
    Input,
    Button,
    InputGroupText,
    Container,
    Row,
    Col,
  } from "@sveltestrap/sveltestrap";

  export let username; // empty string if auth. is disabled, otherwise the username as string
  export let authlevel; // Integer
  export let roles; // Role Enum-Like
  export let screenSize; // screensize
</script>

<Nav navbar>
  {#if screenSize >= 768}
    <NavItem>
      <form method="GET" action="/search">
        <InputGroup>
          <Input
            type="text"
            placeholder="Search 'type:<query>' ..."
            name="searchId"
            style="margin-left: 10px;"
          />
          <!-- bootstrap classes w/o effect -->
          <Button outline type="submit" title="Search"
            ><Icon name="search" /></Button
          >
          <InputGroupText
            style="cursor:help;"
            title={authlevel >= roles.support
              ? "Example: 'projectId:a100cd', Types are: jobId | jobName | projectId | arrayJobId | username | name"
              : "Example: 'jobName:myjob', Types are jobId | jobName | projectId | arrayJobId "}
            ><Icon name="info-circle" /></InputGroupText
          >
        </InputGroup>
      </form>
    </NavItem>
    <NavItem>
      <a
        href="https://www.clustercockpit.org/docs/webinterface/"
        title="Documentation"
        rel="nofollow"
        target="_blank"
      >
        <Button outline style="margin-left: 10px;">
          <Icon name="book" />
        </Button>
      </a>
    </NavItem>
    <NavItem>
      <Button
        outline
        on:click={() => (window.location.href = "/config")}
        style="margin-left: 10px;"
        title="Settings"
      >
        <Icon name="gear" />
      </Button>
    </NavItem>
    {#if username}
      <NavItem>
        <form method="POST" action="/logout">
          <Button
            outline
            color="success"
            type="submit"
            style="margin-left: 10px;"
            title="Logout {username}"
          >
            {#if screenSize > 1630}
              <Icon name="box-arrow-right" /> Logout {username}
            {:else}
              <Icon name="box-arrow-right" />
            {/if}
          </Button>
        </form>
      </NavItem>
    {/if}
  {:else}
    <NavItem>
      <Container>
        <Row cols={3}>
          <Col xs="4">
            <a
              href="https://www.clustercockpit.org/docs/webinterface/"
              title="Documentation"
              rel="nofollow"
              target="_blank"
            >
              <Button outline size="sm" class="my-2 w-100">
                <Icon name="box-arrow-up-right" /> Documentation
              </Button>
            </a>
          </Col>
          <Col xs="4">
            <Button
              outline
              on:click={() => (window.location.href = "/config")}
              size="sm"
              class="my-2 w-100"
            >
              {#if authlevel >= roles.admin}
                <Icon name="gear" /> Admin Settings
              {:else}
                <Icon name="gear" /> Plotting Options
              {/if}
            </Button>
          </Col>
          <Col xs="4">
            <form method="POST" action="/logout">
              <Button
                outline
                color="success"
                type="submit"
                size="sm"
                class="my-2 w-100"
              >
                <Icon name="box-arrow-right" /> Logout {username}
              </Button>
            </form>
          </Col>
        </Row>
      </Container>
    </NavItem>
    <NavItem style="margin-left: 10px; margin-right:10px;">
      <form method="GET" action="/search">
        <InputGroup>
          <Input
            type="text"
            placeholder="Search 'type:<query>' ..."
            name="searchId"
          />
          <Button outline type="submit"><Icon name="search" /></Button>
          <InputGroupText
            style="cursor:help;"
            title={authlevel >= roles.support
              ? "Example: 'projectId:a100cd', Types are: jobId | jobName | projectId | arrayJobId | username | name"
              : "Example: 'jobName:myjob', Types are jobId | jobName | projectId | arrayJobId "}
            ><Icon name="info-circle" /></InputGroupText
          >
        </InputGroup>
      </form>
    </NavItem>
  {/if}
</Nav>
