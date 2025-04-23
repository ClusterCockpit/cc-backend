<!--
    @component Tag List Svelte Component. Displays All Tags, Allows deletion.

    Properties:
    - `username String!`: Users username.
    - `isAdmin Bool!`: User has Admin Auth.
    - `tagmap Object!`: Map of accessible, appwide tags. Prefiltered in backend.
 -->

<script>
  import {
    gql,
    getContextClient,
    mutationStore,
  } from "@urql/svelte";
  import {
    Badge,
    InputGroup,
    Icon,
    Button,
    Spinner,
  } from "@sveltestrap/sveltestrap";
  import {
    init,
  } from "./generic/utils.js";

  export let username;
  export let isAdmin;
  export let tagmap;

  const {} = init();
  const client = getContextClient();

  let pendingChange = "none";

  const removeTagMutation = ({ tagIds }) => {
    return mutationStore({
      client: client,
      query: gql`
        mutation ($tagIds: [ID!]!) {
          removeTagFromList(tagIds: $tagIds)
        }
      `,
      variables: { tagIds },
    });
  };

  function removeTag(tag, tagType) {
    if (confirm("Are you sure you want to completely remove this tag?\n\n" + tagType + ':' + tag.name)) {
      pendingChange = tagType;
      removeTagMutation({tagIds: [tag.id] }).subscribe(
        (res) => {
          if (res.fetching === false && !res.error) {
            tagmap[tagType] = tagmap[tagType].filter((t) => !res.data.removeTagFromList.includes(t.id));
            if (tagmap[tagType].length === 0) {
              delete tagmap[tagType]
            }
            pendingChange = "none";
          } else if (res.fetching === false && res.error) {
            throw res.error;
          }
        },
      );
    }
  }
</script>

<div class="container">
  <div class="row justify-content-center">
    <div class="col-10">
      {#each Object.entries(tagmap) as [tagType, tagList]}
        <div class="my-3 p-2 bg-secondary rounded text-white"> <!-- text-capitalize -->
          Tag Type: <b>{tagType}</b>
          {#if pendingChange === tagType}
            <Spinner size="sm" secondary />
          {/if}
          <span style="float: right; padding-bottom: 0.4rem; padding-top: 0.4rem;" class="badge bg-light text-secondary">
            {tagList.length} Tag{(tagList.length != 1)?'s':''}
          </span>
        </div>
        <div class="d-inline-flex flex-wrap">
          {#each tagList as tag (tag.id)}
            {#if tag.scope == "global"}
              <InputGroup class="w-auto flex-nowrap" style="margin-right: 0.5rem; margin-bottom: 0.5rem;">
                <Button outline color="secondary" href="/monitoring/jobs/?tag={tag.id}" target="_blank">
                  <Badge color="light" style="font-size:medium;" border>{tag.name}</Badge> : 
                  <Badge color="primary" pill>{tag.count} Job{(tag.count != 1)?'s':''}</Badge>
                  <Badge style="background-color:#c85fc8 !important;" pill>Global</Badge>
                </Button>
                {#if isAdmin}
                  <Button
                    size="sm"
                    color="danger"
                    on:click={() => removeTag(tag, tagType)}
                  >
                    <Icon name="x" />
                  </Button>
                {/if}
              </InputGroup>
            {:else if tag.scope == "admin"}
              <InputGroup class="w-auto flex-nowrap" style="margin-right: 0.5rem; margin-bottom: 0.5rem;">
                <Button outline color="secondary" href="/monitoring/jobs/?tag={tag.id}" target="_blank">
                  <Badge color="light" style="font-size:medium;" border>{tag.name}</Badge> : 
                  <Badge color="primary" pill>{tag.count} Job{(tag.count != 1)?'s':''}</Badge>
                  <Badge style="background-color:#19e5e6 !important;" pill>Admin</Badge>
                </Button>
                {#if isAdmin}
                  <Button
                    size="sm"
                    color="danger"
                    on:click={() => removeTag(tag, tagType)}
                  >
                    <Icon name="x" />
                  </Button>
                {/if}
              </InputGroup>
            {:else}
              <InputGroup class="w-auto flex-nowrap" style="margin-right: 0.5rem; margin-bottom: 0.5rem;">
                <Button outline color="secondary" href="/monitoring/jobs/?tag={tag.id}" target="_blank">
                  <Badge color="light" style="font-size:medium;" border>{tag.name}</Badge> : 
                  <Badge color="primary" pill>{tag.count} Job{(tag.count != 1)?'s':''}</Badge>
                  <Badge color="warning" pill>Private</Badge>
                </Button>
                {#if tag.scope == username}
                  <Button
                    size="sm"
                    color="danger"
                    on:click={() => removeTag(tag, tagType)}
                  >
                    <Icon name="x" />
                  </Button>
                {/if}
              </InputGroup>
            {/if}
          {/each}
        </div>
      {/each}
    </div>
  </div>
</div>
