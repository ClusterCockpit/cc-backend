# How to run this as a systemd deamon

The files in this directory assume that you install the Golang version of ClusterCockpit to `/var/clustercockpit`. If you do not like that, you can choose any other location, but make sure to replace all paths that begin with `/var/clustercockpit` in the `clustercockpit.service` file!

If you have not installed [yarn](https://yarnpkg.com/getting-started/install) and [go](https://go.dev/doc/install) already, do that (Golang is available in most package managers).

The `config.json` can have the optional fields *user* and *group*. If provided, the application will call [setuid](https://man7.org/linux/man-pages/man2/setuid.2.html) and [setgid](https://man7.org/linux/man-pages/man2/setgid.2.html) after having read the config file and having bound to a TCP port (so that it can take a privileged port), but before it starts accepting any connections. This is good for security, but means that the directories `frontend/public`, `var/` and `templates/` must be readable by that user and `var/` writable as well (All paths relative to the repos root). The `.env` and `config.json` files might contain secrets and should not be readable by that user. If those files are changed, the server has to be restarted.

```sh
# 1.: Clone this repository to /var/clustercockpit
git clone git@github.com:ClusterCockpit/cc-specifications.git /var/clustercockpit

# 2.: Install all dependencies and build everything
cd /var/clustercockpit
go get && go build && (cd ./frontend && yarn install && yarn build)

# 3.: Modify the `./config.json` file from the directory which contains this README.md to your liking and put it in the repo root
cp ./utils/systemd/config.json ./config.json
vim ./config.json # do your thing...

# 4.: Add the systemd service unit file
sudo ln -s /var/clustercockpit/utils/systemd/clustercockpit.service /etc/systemd/system/clustercockpit.service

# 5.: Enable and start the server
sudo systemctl enable clustercockpit.service # optional (if done, (re-)starts automatically)
sudo systemctl start clustercockpit.service

# Check whats going on:
sudo journalctl -u clustercockpit.service
```
