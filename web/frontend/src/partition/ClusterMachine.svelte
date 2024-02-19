<script>
  import {
    Row,
    Col,
    Card,
    CardBody,
    CardTitle,
    Button,
    Modal,
    ModalHeader,
    ModalBody,
    ModalFooter,
    Input,
  } from "sveltestrap";

  let isModalOpen = false;
  let selectedCard = null;
  let search = "";

  const toggleModal = (card) => {
    selectedCard = card;
    isModalOpen = !isModalOpen;
  };

  let cards = [
    {
      title: "Card 1",
      content: "This is the first card.",
      modalContent: "This is the first modal.",
    },
    {
      title: "Card 2",
      content: "This is the second card.",
      modalContent: "This is the second modal.",
    },
    {
      title: "Card 3",
      content: "This is the third card.",
      modalContent: "This is the third modal.",
    },
  ];
</script>


<Input type="search" placeholder="Search..." bind:value={search} class="mb-2"/>
<Row>
  {#each cards.filter((card) => card.title
        .toLowerCase()
        .includes(search.toLowerCase()) || card.content
        .toLowerCase()
        .includes(search.toLowerCase())) as card (card.title)}
    <Col sm={12} md={6} lg={4}>
      <Card class="m-2">
        <CardBody>
          <CardTitle>{card.title}</CardTitle>
          <p>{card.content}</p>
          <Button on:click={() => toggleModal(card)}>Open Modal</Button>
        </CardBody>
      </Card>
    </Col>
  {/each}
</Row>

<Modal isOpen={isModalOpen} toggle={toggleModal}>
  <ModalHeader toggle={toggleModal}>Modal Title</ModalHeader>
  <ModalBody>{selectedCard ? selectedCard.modalContent : ""}</ModalBody>
  <ModalFooter>
    <Button color="secondary" on:click={toggleModal}>Close</Button>
  </ModalFooter>
</Modal>
