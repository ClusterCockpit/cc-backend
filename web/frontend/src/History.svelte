<script>
  import {
    Col,
    Row,
    Card,
    CardHeader,
    CardBody,
    CardTitle,
    CardSubtitle,
    CardText,
    InputGroup,
    InputGroupText,
    Input,
    Button,
    Icon,
    Dropdown,
    DropdownToggle,
    DropdownMenu,
    DropdownItem,
  } from "sveltestrap";
  import SvTable from "./partition/SvTable.svelte";
  let pages = [5, 10, 15, 20];
  let selectedPages = 5;
  let isOpen = false;

  const selectPage = (page) => {
    selectedPages = page;
    isOpen = false;
  };
</script>

<Col cols={1} class="p-2 g-2">
  <Row>
    <Col>
      <Card class="mb-1">
        <CardHeader>
          <CardTitle>
            <h3>History</h3></CardTitle
          >
        </CardHeader>
        <CardBody class="h5">
          <CardSubtitle
            >History records for past connections are listed here and can be
            sorted by clicking the column headers. To search for specific
            records, enter a filter string and click "Search". Only records
            which match the provided filter string will be listed.
          </CardSubtitle>
          <CardText></CardText>
          <Row>
            <Row class="p-3">
              <Col>
                <!-- bind:value={filterString} -->
                <InputGroup>
                  <Input type="text" placeholder="Filter string" />
                  <InputGroupText>
                    <Icon name="search" />
                  </InputGroupText>
                </InputGroup>
              </Col>
              <Col>
                <!-- <Button color="primary" on:click={search}> -->
                <Button color="primary">Search</Button>
              </Col>
            </Row>
          </Row>
          <!-- <br> -->
          <Row class="p-3">
            <SvTable />
          </Row>
          <!-- pagination -->
          <Row>
            <Col cols="3">
              <!-- <Button color="primary" on:click={prevPage}> -->
              <Button color="primary">Previous</Button>
              <!-- <Button color="primary" on:click={nextPage}> -->
              <Button color="primary">Next</Button>
              <!-- dropdown for number of pages to  show -->
              <!-- <div> -->
                <Dropdown {isOpen} toggle={() => (isOpen = !isOpen)}>
                    <DropdownToggle caret>
                      {selectedPages}
                    </DropdownToggle>
                    <DropdownMenu>
                      {#each pages as page (page)}
                        <DropdownItem on:click={() => selectPage(page)}
                          >{page}</DropdownItem
                        >
                      {/each}
                    </DropdownMenu>
                  </Dropdown>
              <!-- </div> -->
            </Col>
          </Row>
        </CardBody>
      </Card>
    </Col>
  </Row>
</Col>
