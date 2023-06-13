# Overview

Customizing `cc-backend` means changing the logo and certain legal texts
instead of the placeholders. To change the logo displayed in the navigation bar, the
file `web/frontend/public/img/logo.png` in the source tree must be replaced
and cc-backend must be rebuild.

# Replace legal texts

To replace the `imprint.tmpl` and `privacy.tmpl` legal texts, you can place your
version in `./var/`. At startup `cc-backend` will check if `./var/imprint.tmpl` and/or
`./var/privacy.tmpl` exist and use them instead of the built-in placeholders.
You can use the placeholders in `web/templates` as a blueprint.
