# Overview

To customize `cc-backend` means to change the logo and add specific legal texts
in place of the placeholders. To change the logo shown in the navigation bar the
file `web/frontend/public/img/logo.png` has to be replaced in the source tree
and cc-backend has to be rebuild.

# Replace legal texts

To replace the legal texts `imprint.tmpl` and `privacy.tmpl` you can place your
version in `./var/`. On startup `cc-backend` will check if `./var/imprint.tmpl` and/or
`./var/privacy.tmpl` exist and use those instead of the builtin placeholders.
You may use the placeholders in `web/templates` as blueprint.
