<!--
    @component Tag List Svelte Component. Displays All Tags, Allows deletion.

    Properties:
    - `authlevel Int!`: Current Users Authority Level
    - `tagmap Object!`: Map of Appwide Tags
 -->

<script>
  // import { Card, CardHeader, CardTitle } from "@sveltestrap/sveltestrap";

  // export let authlevel;
  export let tagmap;
</script>

<div class="container">
    <div class="row justify-content-center">
        <div class="col-10">
        {#each Object.entries(tagmap) as [tagType, tagList]}
            <div class="my-3 p-2 bg-secondary rounded text-white"> <!-- text-capitalize -->
                Tag Type: <b>{tagType}</b>
                <span style="float: right; padding-bottom: 0.4rem; padding-top: 0.4rem;" class="badge bg-light text-secondary">
                    {tagList.length} Tag{(tagList.length != 1)?'s':''}
                </span>
            </div>
            {#each tagList as tag (tag.id)}
                {#if tag.scope == "global"}
                    <a class="btn btn-outline-secondary" href="/monitoring/jobs/?tag={tag.id}" role="button">
                        {tag.name}
                        <span class="badge bg-primary mr-1">{tag.count} Job{(tag.count != 1)?'s':''}</span>
                        <span style="background-color:#c85fc8;" class="badge text-dark">Global</span>
                    </a>
                {:else if tag.scope == "admin"}
                    <a class="btn btn-outline-secondary" href="/monitoring/jobs/?tag={tag.id}" role="button">
                        {tag.name}
                        <span class="badge bg-primary mr-1">{tag.count} Job{(tag.count != 1)?'s':''}</span>
                        <span style="background-color:#19e5e6;" class="badge text-dark">Admin</span>
                    </a>
                {:else}
                    <a class="btn btn-outline-secondary" href="/monitoring/jobs/?tag={tag.id}" role="button">
                        {tag.name}
                        <span class="badge bg-primary mr-1">{tag.count} Job{(tag.count != 1)?'s':''}</span>
                        <span class="badge bg-warning text-dark">Private</span>
                    </a>
                {/if}
            {/each}
        {/each}
        </div>
    </div>
</div>
