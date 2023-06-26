## Intro

cc-backend requires a configuration file that specifies the cluster systems to be used.
To override the default, specify the location of a json configuration file with the `-config <file path>` command line option.
All security-related configurations, e.g. keys and passwords, are set using
environment variables.
It is supported to set these by means of a `.env` file in the project root.

## Configuration Options

* `addr`: Type string.  Address where the http (or https) server will listen on (for example: 'localhost:80'). Default `:8080`.
* `user`: Type string. Drop root permissions once .env was read and the port was taken. Only applicable if using privileged port.
* `group`: Type string.  Drop root permissions once .env was read and the port was taken. Only applicable if using privileged port.
* `disable-authentication`: Type bool.  Disable authentication (for everything: API, Web-UI, ...). Default `false`.
* `embed-static-files`: Type bool. If all files in `web/frontend/public` should be served from within the binary itself (they are embedded) or not. Default `true`.
* `static-files`: Type string. Folder where static assets can be found, if `embed-static-files` is `false`. No default.
* `db-driver`: Type string. 'sqlite3' or 'mysql' (mysql will work for mariadb as well). Default `sqlite3`.
* `db`: Type string. For sqlite3 a filename, for mysql a DSN in this format: https://github.com/go-sql-driver/mysql#dsn-data-source-name (Without query parameters!). Default: `./var/job.db`.
* `job-archive`: Type string. Path to the job-archive. Default: `./var/job-archive`.
* `disable-archive`: Type bool. Keep all metric data in the metric data repositories, do not write to the job-archive. Default `false`.
* `validate`: Type bool. Validate all input json documents against json schema.
* `session-max-age`: Type string. Specifies for how long a session shall be valid  as a string parsable by time.ParseDuration(). If 0 or empty, the session/token does not expire! Default `168h`.
* `jwt-max-age`: Type string. Specifies for how long a JWT token shall be valid  as a string parsable by time.ParseDuration(). If 0 or empty, the session/token does not expire! Default `0`.
* `https-cert-file` and `https-key-file`: Type string. If both those options are not empty, use HTTPS using those certificates.
* `redirect-http-to`: Type string. If not the empty string and `addr` does not end in ":80", redirect every request incoming at port 80 to that url.
* `machine-state-dir`: Type string. Where to store MachineState files. TODO: Explain in more detail!
* `stop-jobs-exceeding-walltime`: Type int. If not zero, automatically mark jobs as stopped running X seconds longer than their walltime. Only applies if walltime is set for job. Default `0`.
* `short-running-jobs-duration`: Type int. Do not show running jobs shorter than X seconds. Default `300`.
* `ldap`: Type object. For LDAP Authentication and user synchronisation. Default `nil`.
   - `url`: Type string.  URL of LDAP directory server.
   - `user_base`: Type string. Base DN of user tree root.
   - `search_dn`: Type string. DN for authenticating LDAP admin account with general read rights.
   - `user_bind`: Type string. Expression used to authenticate users via LDAP bind. Must contain `uid={username}`.
   - `user_filter`: Type string. Filter to extract users for syncing.
   - `sync_interval`: Type string. Interval used for syncing local user table with LDAP directory. Parsed using time.ParseDuration.
   - `sync_del_old_users`: Type bool. Delete obsolete users in database.
* `clusters`: Type array of objects
   - `name`: Type string. The name of the cluster.
   - `metricDataRepository`: Type object with properties: `kind` (Type string, can be one of `cc-metric-store`, `influxdb` ), `url` (Type string), `token` (Type string)
   - `filterRanges` Type object. This option controls the slider ranges for the UI controls of numNodes, duration, and startTime.  Example:
   ```
   "filterRanges": {
                "numNodes": { "from": 1, "to": 64 },
                "duration": { "from": 0, "to": 86400 },
                "startTime": { "from": "2022-01-01T00:00:00Z", "to": null }
            }
   ```
* `ui-defaults`: Type object. Default configuration for ui views. If overwritten, all options  must be provided! Most options can be overwritten by the user via the web interface.
   - `analysis_view_histogramMetrics`: Type string array. Metrics to show as job count histograms in analysis view. Default `["flops_any", "mem_bw", "mem_used"]`.
   - `analysis_view_scatterPlotMetrics`: Type array of string array. Initial
   scatter plot configuration in analysis view. Default `[["flops_any", "mem_bw"], ["flops_any", "cpu_load"], ["cpu_load", "mem_bw"]]`.
   - `job_view_nodestats_selectedMetrics`: Type string array. Initial metrics shown in node statistics table of single job view. Default `["flops_any", "mem_bw", "mem_used"]`.
   - `job_view_polarPlotMetrics`: Type string array. Metrics shown in polar plot of single job view. Default `["flops_any", "mem_bw", "mem_used", "net_bw", "file_bw"]`.
   - `job_view_selectedMetrics`: Type string array.  Default `["flops_any", "mem_bw", "mem_used"]`.
   - `plot_general_colorBackground`: Type bool. Color plot background according to job average threshold limits. Default `true`.
   - `plot_general_colorscheme`: Type string array. Initial color scheme. Default `"#00bfff", "#0000ff", "#ff00ff", "#ff0000", "#ff8000", "#ffff00", "#80ff00"`.
   - `plot_general_lineWidth`: Type int. Initial linewidth. Default `3`.
   - `plot_list_jobsPerPage`: Type int. Jobs shown per page in job lists. Default `50`.
   - `plot_list_selectedMetrics`: Type string array. Initial metric plots shown in jobs lists. Default `"cpu_load", "ipc", "mem_used", "flops_any", "mem_bw"`.
   - `plot_view_plotsPerRow`: Type int. Number of plots per row in single job view. Default `3`.
   - `plot_view_showPolarplot`: Type bool. Option to toggle polar plot in single job view. Default `true`.
   - `plot_view_showRoofline`: Type bool. Option to toggle roofline plot in single job view. Default `true`.
   - `plot_view_showStatTable`: Type bool. Option to toggle the node statistic table in single job view. Default `true`.
   - `system_view_selectedMetric`: Type string. Initial metric shown in system view. Default `cpu_load`.

Some of the `ui-defaults` values can be appended by `:<clustername>` in order to have different settings depending on the current cluster. Those are notably `job_view_nodestats_selectedMetrics`, `job_view_polarPlotMetrics`, `job_view_selectedMetrics` and `plot_list_selectedMetrics`.

## Environment Variables

An example env file is found in this directory. Copy it to `.env` in the project root and adapt it for your needs.

* `JWT_PUBLIC_KEY` and `JWT_PRIVATE_KEY`: Base64 encoded Ed25519 keys used for JSON Web Token (JWT) authentication. You can generate your own keypair using `go run ./cmd/gen-keypair/gen-keypair.go`. More information in [README_TOKENS.md](./README_TOKENS.md).
* `SESSION_KEY`: Some random bytes used as secret for cookie-based sessions.
* `LDAP_ADMIN_PASSWORD`: The LDAP admin user password (optional).
* `CROSS_LOGIN_JWT_HS512_KEY`: Used for token based logins via another authentication service.
* `LOGLEVEL`: Can be `err`, `warn`, `info` or `debug` (optional, `warn` by default). Can be used to reduce logging.
