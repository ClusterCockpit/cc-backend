# cc-frontend

[![Build](https://github.com/ClusterCockpit/cc-svelte-datatable/actions/workflows/build.yml/badge.svg)](https://github.com/ClusterCockpit/cc-svelte-datatable/actions/workflows/build.yml)

A frontend for [ClusterCockpit](https://github.com/ClusterCockpit/ClusterCockpit) and [cc-backend](https://github.com/ClusterCockpit/cc-backend). Backend specific configuration can de done using the constants defined in the `intro` section in `./rollup.config.js`.

Builds on:
* [Svelte](https://svelte.dev/)
* [SvelteStrap](https://sveltestrap.js.org/)
* [Bootstrap 5](https://getbootstrap.com/)
* [urql](https://github.com/FormidableLabs/urql)

## Get started

[Yarn](https://yarnpkg.com/) is recommended for package management.
Due to an issue with Yarn v2 you have to stick to Yarn v1.

Install the dependencies...

```bash
yarn install
```

...then start [Rollup](https://rollupjs.org):

```bash
yarn run dev
```

Edit a component file in `src`, save it, and reload the page to see your changes.

