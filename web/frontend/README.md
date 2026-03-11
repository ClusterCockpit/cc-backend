# cc-frontend

[![Build](https://github.com/ClusterCockpit/cc-backend/actions/workflows/test.yml/badge.svg)](https://github.com/ClusterCockpit/cc-backend/actions/workflows/test.yml)

A frontend for [ClusterCockpit](https://github.com/ClusterCockpit/ClusterCockpit) and [cc-backend](https://github.com/ClusterCockpit/cc-backend). Backend specific configuration can be done using the constants defined in the `intro` section in `./rollup.config.mjs`.

Builds on:
* [Svelte 5](https://svelte.dev/)
* [SvelteStrap](https://sveltestrap.js.org/)
* [Bootstrap 5](https://getbootstrap.com/)
* [urql](https://github.com/FormidableLabs/urql)

## Get started

Install the dependencies...

```bash
npm install
```

...then build using [Rollup](https://rollupjs.org):

```bash
npm run build
```

