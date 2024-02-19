<script>
  import { Icon, TabContent, TabPane } from "sveltestrap";
    import LinuxUser from "./LinuxUser.svelte";
</script>

<TabContent>
  <TabPane tabId="local-user" active class="mt-3">
    <span slot="tab">
      Local User
      <!-- <Icon name="gear" /> -->
    </span>
  </TabPane>
  <TabPane tabId="linux-user" class="mt-3">
    <span slot="tab">
      Linux User
      <!-- <Icon name="hand-thumbs-up" /> -->
    </span>
    <LinuxUser />
  </TabPane>
  <TabPane tabId="group" class="mt-3">
    <span slot="tab">
      Group
      <!-- <Icon name="alarm" /> -->
    </span>
  </TabPane>
</TabContent>
