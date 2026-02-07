<!--
  @component Systemd Journal Log Viewer (Admin only)

  Properties:
  - `isAdmin Bool!`: Is currently logged in user admin authority
-->

<script>
  import {
    Card,
    CardHeader,
    CardBody,
    Table,
    Input,
    Button,
    Badge,
    Spinner,
    InputGroup,
    InputGroupText,
    Icon,
  } from "@sveltestrap/sveltestrap";

  let { isAdmin } = $props();

  const timeRanges = [
    { label: "Last 15 minutes", value: "15 min ago" },
    { label: "Last 1 hour", value: "1 hour ago" },
    { label: "Last 6 hours", value: "6 hours ago" },
    { label: "Last 24 hours", value: "24 hours ago" },
    { label: "Last 7 days", value: "7 days ago" },
  ];

  const levels = [
    { label: "All levels", value: "" },
    { label: "Emergency (0)", value: "0" },
    { label: "Alert (1)", value: "1" },
    { label: "Critical (2)", value: "2" },
    { label: "Error (3)", value: "3" },
    { label: "Warning (4)", value: "4" },
    { label: "Notice (5)", value: "5" },
    { label: "Info (6)", value: "6" },
    { label: "Debug (7)", value: "7" },
  ];

  const refreshIntervals = [
    { label: "Off", value: 0 },
    { label: "5s", value: 5000 },
    { label: "10s", value: 10000 },
    { label: "30s", value: 30000 },
  ];

  let since = $state("1 hour ago");
  let level = $state("");
  let search = $state("");
  let linesParam = $state("200");
  let refreshInterval = $state(0);
  let entries = $state([]);
  let loading = $state(false);
  let error = $state(null);
  let timer = $state(null);

  function levelColor(priority) {
    if (priority <= 2) return "danger";
    if (priority === 3) return "warning";
    if (priority === 4) return "info";
    if (priority <= 6) return "secondary";
    return "light";
  }

  function levelName(priority) {
    const names = ["EMERG", "ALERT", "CRIT", "ERR", "WARN", "NOTICE", "INFO", "DEBUG"];
    return names[priority] || "UNKNOWN";
  }

  function formatTimestamp(usec) {
    if (!usec) return "";
    const ms = parseInt(usec) / 1000;
    const d = new Date(ms);
    return d.toLocaleString();
  }

  async function fetchLogs() {
    loading = true;
    error = null;
    try {
      const params = new URLSearchParams();
      params.set("since", since);
      params.set("lines", linesParam);
      if (level) params.set("level", level);
      if (search.trim()) params.set("search", search.trim());

      const resp = await fetch(`/config/logs/?${params.toString()}`);
      if (!resp.ok) {
        const body = await resp.json();
        throw new Error(body.error || `HTTP ${resp.status}`);
      }
      entries = await resp.json();
    } catch (e) {
      error = e.message;
      entries = [];
    } finally {
      loading = false;
    }
  }

  function setupAutoRefresh(interval) {
    if (timer) {
      clearInterval(timer);
      timer = null;
    }
    if (interval > 0) {
      timer = setInterval(fetchLogs, interval);
    }
  }

  $effect(() => {
    setupAutoRefresh(refreshInterval);
    return () => {
      if (timer) clearInterval(timer);
    };
  });

  // Fetch on mount
  $effect(() => {
    fetchLogs();
  });
</script>

{#if !isAdmin}
  <Card>
    <CardBody>
      <p>Access denied. Admin privileges required.</p>
    </CardBody>
  </Card>
{:else}
  <Card class="mb-3">
    <CardHeader>
      <div class="d-flex flex-wrap align-items-center gap-2">
        <InputGroup size="sm" style="max-width: 200px;">
          <Input type="select" bind:value={since}>
            {#each timeRanges as tr}
              <option value={tr.value}>{tr.label}</option>
            {/each}
          </Input>
        </InputGroup>

        <InputGroup size="sm" style="max-width: 180px;">
          <Input type="select" bind:value={level}>
            {#each levels as lv}
              <option value={lv.value}>{lv.label}</option>
            {/each}
          </Input>
        </InputGroup>

        <InputGroup size="sm" style="max-width: 150px;">
          <InputGroupText>Lines</InputGroupText>
          <Input type="select" bind:value={linesParam}>
            <option value="100">100</option>
            <option value="200">200</option>
            <option value="500">500</option>
            <option value="1000">1000</option>
          </Input>
        </InputGroup>

        <InputGroup size="sm" style="max-width: 250px;">
          <Input
            type="text"
            placeholder="Search..."
            bind:value={search}
            onkeydown={(e) => { if (e.key === "Enter") fetchLogs(); }}
          />
        </InputGroup>

        <Button size="sm" color="primary" onclick={fetchLogs} disabled={loading}>
          {#if loading}
            <Spinner size="sm" />
          {:else}
            <Icon name="arrow-clockwise" />
          {/if}
          Refresh
        </Button>

        <InputGroup size="sm" style="max-width: 140px;">
          <InputGroupText>Auto</InputGroupText>
          <Input type="select" bind:value={refreshInterval}>
            {#each refreshIntervals as ri}
              <option value={ri.value}>{ri.label}</option>
            {/each}
          </Input>
        </InputGroup>

        {#if entries.length > 0}
          <small class="text-muted ms-auto">{entries.length} entries</small>
        {/if}
      </div>
    </CardHeader>
    <CardBody style="padding: 0;">
      {#if error}
        <div class="alert alert-danger m-3">{error}</div>
      {/if}

      <div style="max-height: 75vh; overflow-y: auto;">
        <Table size="sm" striped hover responsive class="mb-0">
          <thead class="sticky-top bg-white">
            <tr>
              <th style="width: 170px;">Timestamp</th>
              <th style="width: 80px;">Level</th>
              <th>Message</th>
            </tr>
          </thead>
          <tbody style="font-family: monospace; font-size: 0.85rem;">
            {#each entries as entry}
              <tr>
                <td class="text-nowrap">{formatTimestamp(entry.timestamp)}</td>
                <td><Badge color={levelColor(entry.priority)}>{levelName(entry.priority)}</Badge></td>
                <td style="white-space: pre-wrap; word-break: break-all;">{entry.message}</td>
              </tr>
            {:else}
              {#if !loading && !error}
                <tr><td colspan="3" class="text-center text-muted py-3">No log entries found</td></tr>
              {/if}
            {/each}
          </tbody>
        </Table>
      </div>
    </CardBody>
  </Card>
{/if}
