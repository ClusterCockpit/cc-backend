## Tips for frontend development

The frontend assets including the Svelte js files are per default embedded in
the bgo binary. To enable a quick turnaround cycle for web development of the
frontend disable embedding of static assets in `config.json`:
```
"embed-static-files": false,
"static-files": "./web/frontend/public/",

```

Start the node build process (in directory `./web/frontend`) in development mode:
```
$ npm run dev
```

This will start the build process in listen mode. Whenever you change a source
files the depending javascript targets will be automatically rebuild.
In case the javascript files are minified you may need to set the production
flag by hand to false in `./web/frontend/rollup.config.mjs`:
```
const production = false
```

Usually this should work automatically.

Because the files are still served by ./cc-backend you have to reload the view
explicitly in your browser.

A common setup is to have three terminals open:
* One running cc-backend (working directory repository root): `./cc-backend -server -dev`
* Another running npm in developer mode (working directory `./web/frontend`): `npm run dev`
* And the last one editing the frontend source files
