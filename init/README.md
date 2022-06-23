# How to run this as a systemd deamon

The files in this directory assume that you install ClusterCockpit to `/opt/monitoring`.
Of course you can choose any other location, but make sure to replace all paths that begin with `/opt/monitoring` in the `clustercockpit.service` file!

If you have not installed [yarn](https://yarnpkg.com/getting-started/install) and [go](https://go.dev/doc/install) already, do that (Golang is available in most package managers).
It is recommended and easy to install the most recent stable version of Golang as every version also improves the Golang standard library.

The `config.json` can have the optional fields *user* and *group*.
If provided, the application will call [setuid](https://man7.org/linux/man-pages/man2/setuid.2.html) and [setgid](https://man7.org/linux/man-pages/man2/setgid.2.html) after having read the config file and having bound to a TCP port (so that it can take a privileged port), but before it starts accepting any connections.
This is good for security, but means that the directories `web/frontend/public`, `var/` and `web/templates/` must be readable by that user and `var/` writable as well (All paths relative to the repos root).
The `.env` and `config.json` files might contain secrets and should not be readable by that user.
If those files are changed, the server has to be restarted.

```sh
# 1.: Clone this repository to /opt/monitoring
git clone git@github.com:ClusterCockpit/cc-backend.git /opt/monitoring

# 2.: Install all dependencies and build everything
cd /mnt/monitoring
go get && go build cmd/cc-backend && (cd ./web/frontend && yarn install && yarn build)

# 3.: Modify the `./config.json` and env-template.txt file from the configs directory to your liking and put it in the repo root
cp ./configs/config.json ./config.json
cp ./configs/env-template.txt ./.env
vim ./config.json # do your thing...
vim ./.env # do your thing...

# 4.: Add the systemd service unit file (in case /opt/ is mounted on another file system it may be better to copy the file to /etc)
sudo ln -s /mnt/monitoring/init/clustercockpit.service /etc/systemd/system/clustercockpit.service

# 5.: Enable and start the server
sudo systemctl enable clustercockpit.service # optional (if done, (re-)starts automatically)
sudo systemctl start clustercockpit.service

# Check whats going on:
sudo journalctl -u clustercockpit.service
```
