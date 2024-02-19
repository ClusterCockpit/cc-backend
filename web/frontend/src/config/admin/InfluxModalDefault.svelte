<script>
    import { Button, Modal, ModalHeader, ModalBody, ModalFooter } from 'sveltestrap';
    import InfoxDbConf from './InfluxDbConf.svelte';
    import { onMount } from 'svelte';

    let isOpen = false;
    let influxDbConfig = {};

    onMount(async () => {
        // Fetch the default values of influxdb configuration from db
        const response = await fetch('/api/influxdb-config');
        influxDbConfig = await response.json();
    });

    const toggle = () => {
        isOpen = !isOpen;
    };
</script>

<Button color="primary btn"  style="" on:click={toggle}>Show InfluxDB Configuration</Button>

<Modal isOpen={isOpen} toggle={toggle}>
    <ModalHeader toggle={toggle}>InfluxDB Configuration</ModalHeader>
    <ModalBody style="max-height: calc(100vh - 210px); overflow-y: auto;">
        <InfoxDbConf {influxDbConfig} />
    </ModalBody>
    <ModalFooter>
        <Button color="secondary" on:click={toggle}>Close</Button>
    </ModalFooter>
</Modal>