# How to run `cc-backend` as a systemd service.

The files in this directory assume that you install ClusterCockpit to
`/opt/monitoring/cc-backend`.
Of course you can choose any other location, but make sure you replace all paths
starting with `/opt/monitoring/cc-backend` in the `clustercockpit.service` file!

The `config.json` may contain the optional fields *user* and *group*. If
specified, the application will call
[setuid](https://man7.org/linux/man-pages/man2/setuid.2.html) and
[setgid](https://man7.org/linux/man-pages/man2/setgid.2.html) after reading the
config file and binding to a TCP port (so it can take a privileged port), but
before it starts accepting any connections. This is good for security, but also
means that the `var/` directory must be readable and writeable by this user.
The `.env` and `config.json` files may contain secrets and should not be
readable by this user. If these files are changed, the server must be restarted.

```sh
# 1. Clone this repository somewhere in your home
git clone git@github.com:ClusterCockpit/cc-backend.git <DSTDIR>

# 2. (Optional) Install dependencies and build. In general it is recommended to use the provided release binaries.
cd <DSTDIR>
make
sudo mkdir -p /opt/monitoring/cc-backend/
cp ./cc-backend /opt/monitoring/cc-backend/

# 3. Modify the `./config.json` and env-template.txt file from the configs directory to your liking and put it in the target directory
cp ./configs/config.json /opt/monitoring/config.json
cp ./configs/env-template.txt /opt/monitoring/.env
vim /opt/monitoring/config.json # do your thing...
vim /opt/monitoring/.env # do your thing...

# 4. (Optional) Customization: Add your versions of the login view, legal texts, and logo image.
# You may use the templates in `./web/templates` as blueprint. Every overwrite separate.
cp login.tmpl /opt/monitoring/cc-backend/var/
cp imprint.tmpl /opt/monitoring/cc-backend/var/
cp privacy.tmpl /opt/monitoring/cc-backend/var/
# Ensure your logo, and any images you use in your login template has a suitable size.
cp -R img /opt/monitoring/cc-backend/img

# 5. Copy the systemd service unit file. You may adopt it to your needs.
sudo cp ./init/clustercockpit.service /etc/systemd/system/clustercockpit.service

# 6. Enable and start the server
sudo systemctl enable clustercockpit.service # optional (if done, (re-)starts automatically)
sudo systemctl start clustercockpit.service

# Check whats going on:
sudo systemctl status clustercockpit.service
sudo journalctl -u clustercockpit.service
```

# Recommended workflow for deployment

It is recommended to install all ClusterCockpit components in a common directory, e.g. `/opt/monitoring`, `var/monitoring` or `var/clustercockpit`.
In the following we use `/opt/monitoring`.

Two systemd services run on the central monitoring server:
* clustercockpit : binary cc-backend in `/opt/monitoring/cc-backend`.
* cc-metric-store : Binary cc-metric-store in `/opt/monitoring/cc-metric-store`.

ClusterCockpit is deployed as a single binary that embeds all static assets.
We recommend keeping all `cc-backend` binary versions in a folder `archive` and
linking the currently active one from the `cc-backend` root.
This allows for easy roll-back in case something doesn't work.

## Workflow to deploy new version

This example assumes the DB and job archive versions did not change.
* Stop systemd service: `$ sudo systemctl stop clustercockpit.service`
* Backup the sqlite DB file and Job archive directory tree!
* Copy `cc-backend` binary to `/opt/monitoring/cc-backend/archive` (Tip: Use a
date tag like `YYYYMMDD-cc-backend`)
* Link from cc-backend root to current version
* Start systemd service: `$ sudo systemctl start clustercockpit.service`
* Check if everything is ok: `$ sudo systemctl status clustercockpit.service`
* Check log for issues: `$ sudo journalctl -u clustercockpit.service`
* Check the ClusterCockpit web frontend and your Slurm adapters if anything is broken!
