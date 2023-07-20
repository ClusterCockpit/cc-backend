# Overview

Customizing `cc-backend` means changing the logo, legal texts, and the login
template instead of the placeholders. You can also place a text file in `./var`
to add dynamic status or notification messages to the clusterCockpit homepage.

# Replace legal texts

To replace the `imprint.tmpl` and `privacy.tmpl` legal texts, you can place your
version in `./var/`. At startup `cc-backend` will check if `./var/imprint.tmpl` and/or
`./var/privacy.tmpl` exist and use them instead of the built-in placeholders.
You can use the placeholders in `web/templates` as a blueprint.

# Replace login template
To replace the default login layout and styling, you can place your version in
`./var/`. At startup `cc-backend` will check if `./var/login.tmpl` exist and use
it instead of the built-in placeholder. You can use the default temaplte
`web/templates/login.tmpl` as a blueprint.

# Replace logo
To change the logo displayed in the navigation bar, you can provide the file
`logo.png` in the folder `./var/img/`. On startup `cc-backend` will check if the
folder exists and use the images provided there instead of the built-in images.
You may also place additional images there you use in a custom login template.

# Add notification banner on homepage
To add a notification banner you can add a file `notice.txt` to `./var`. As long
as this file is present all text in this file is shown in an info banner on the
homepage.
