# Hands-on setup ClusterCockpit from scratch (w/o docker)

## Prerequisites
* perl
* go
* npm
* Optional: curl
* Script migrateTimestamp.pl

## Documentation
You find READMEs or api docs in
* ./cc-backend/configs
* ./cc-backend/init
* ./cc-backend/api

## ClusterCockpit configuration files
### cc-backend
* `./.env` Passwords and Tokens set in the environment
* `./config.json` Configuration options for cc-backend

### cc-metric-store
* `./config.json` Optional to overwrite configuration options

### cc-metric-collector
Not yet included in the hands-on setup.

## Setup Components
Start by creating a base folder for all of the following steps.
* `mkdir clustercockpit`
* `cd clustercockpit`

### Setup cc-backend
* Clone Repository
    - `git clone https://github.com/ClusterCockpit/cc-backend.git`
    - `cd cc-backend`
* Build
    - `make`
* Activate & configure environment for cc-backend
    - `cp configs/env-template.txt  .env`
    - Optional: Have a look via `vim .env`
    - Copy the `config.json` file included in this tarball into the root directory of cc-backend: `cp ../../config.json  ./`
* Back to toplevel `clustercockpit`
    - `cd ..`
* Prepare Datafolder and Database file
    - `mkdir var`
    - `./cc-backend -migrate-db`

### Setup cc-metric-store
* Clone Repository
    - `git clone https://github.com/ClusterCockpit/cc-metric-store.git`
    - `cd cc-metric-store`
* Build Go Executable
    - `go get`
    - `go build`
* Prepare Datafolders
    - `mkdir -p var/checkpoints`
    - `mkdir -p var/archive`
* Update Config
    - `vim config.json`
    - Exchange existing setting in `metrics` with the following:
```
"clock":      { "frequency": 60, "aggregation": null },
"cpi":        { "frequency": 60, "aggregation": null },
"cpu_load":   { "frequency": 60, "aggregation": null },
"flops_any":  { "frequency": 60, "aggregation": null },
"flops_dp":   { "frequency": 60, "aggregation": null },
"flops_sp":   { "frequency": 60, "aggregation": null },
"ib_bw":      { "frequency": 60, "aggregation": null },
"lustre_bw":  { "frequency": 60, "aggregation": null },
"mem_bw":     { "frequency": 60, "aggregation": null },
"mem_used":   { "frequency": 60, "aggregation": null },
"rapl_power": { "frequency": 60, "aggregation": null }
```
* Back to toplevel `clustercockpit`
    - `cd ..`

### Setup Demo Data
* `mkdir source-data`
* `cd source-data`
* Download JobArchive-Source:
    - `wget https://hpc-mover.rrze.uni-erlangen.de/HPC-Data/0x7b58aefb/eig7ahyo6fo2bais0ephuf2aitohv1ai/job-archive-dev.tar.xz`
    - `tar xJf job-archive-dev.tar.xz`
    - `mv ./job-archive ./job-archive-source`
    - `rm ./job-archive-dev.tar.xz`
* Download CC-Metric-Store Checkpoints:
    - `mkdir -p cc-metric-store-source/checkpoints`
    - `cd cc-metric-store-source/checkpoints`
    - `wget https://hpc-mover.rrze.uni-erlangen.de/HPC-Data/0x7b58aefb/eig7ahyo6fo2bais0ephuf2aitohv1ai/cc-metric-store-checkpoints.tar.xz`
    - `tar xf cc-metric-store-checkpoints.tar.xz`
    - `rm cc-metric-store-checkpoints.tar.xz`
* Back to `source-data`
    - `cd ../..`
* Run timestamp migration script. This may take tens of minutes!
    - `cp ../migrateTimestamps.pl .`
    - `./migrateTimestamps.pl`
    - Expected output:
```
Starting to update start- and stoptimes in job-archive for emmy
Starting to update start- and stoptimes in job-archive for woody
Done for job-archive
Starting to update checkpoint filenames and data starttimes for emmy
Starting to update checkpoint filenames and data starttimes for woody
Done for checkpoints
```
* Copy `cluster.json` files from source to migrated folders
    - `cp source-data/job-archive-source/emmy/cluster.json cc-backend/var/job-archive/emmy/`
    - `cp source-data/job-archive-source/woody/cluster.json cc-backend/var/job-archive/woody/`
* Initialize Job-Archive in SQLite3 job.db and add demo user
    - `cd cc-backend`
    - `./cc-backend -init-db -add-user demo:admin:demo`
    - Expected output:
```
<6>[INFO]    new user "demo" created (roles: ["admin"], auth-source: 0)
<6>[INFO]    Building job table...
<6>[INFO]    A total of 3936 jobs have been registered in 1.791 seconds.
```
* Back to toplevel `clustercockpit`
    - `cd ..`

### Startup both Apps
* In cc-backend root:  `$./cc-backend -server -dev`
    - Starts Clustercockpit at `http:localhost:8080`
        - Log: `<6>[INFO]    HTTP server listening at :8080...`
    - Use local internet browser to access interface
        - You should see and be able to browse finished Jobs
        - Metadata is read from SQLite3 database
        - Metricdata is read from job-archive/JSON-Files
    - Create User in settings (top-right corner)
        - Name `apiuser`
        - Username `apiuser`
        - Role `API`
        - Submit & Refresh Page
    - Create JTW for `apiuser`
        - In Userlist, press `Gen. JTW` for `apiuser`
        - Save JWT for later use
* In cc-metric-store root:  `$./cc-metric-store`
    - Start the cc-metric-store on `http:localhost:8081`, Log:
```
2022/07/15 17:17:42 Loading checkpoints newer than 2022-07-13T17:17:42+02:00
2022/07/15 17:17:45 Checkpoints loaded (5621 files, 319 MB, that took 3.034652s)
2022/07/15 17:17:45 API http endpoint listening on '0.0.0.0:8081'
```
    - Does *not* have a graphical interface
    - Otpional: Test function by executing:
```
$ curl -H "Authorization: Bearer eyJ0eXAiOiJKV1QiLCJhbGciOiJFZERTQSJ9.eyJ1c2VyIjoiYWRtaW4iLCJyb2xlcyI6WyJST0xFX0FETUlOIiwiUk9MRV9BTkFMWVNUIiwiUk9MRV9VU0VSIl19.d-3_3FZTsadPjDEdsWrrQ7nS0edMAR4zjl-eK7rJU3HziNBfI9PDHDIpJVHTNN5E5SlLGLFXctWyKAkwhXL-Dw" -D - "http://localhost:8081/api/query" -d "{ \"cluster\": \"emmy\", \"from\": $(expr $(date +%s) - 60), \"to\": $(date +%s), \"queries\": [{
  \"metric\": \"flops_any\",
  \"host\": \"e1111\"
}] }"

HTTP/1.1 200 OK
Content-Type: application/json
Date: Fri, 15 Jul 2022 13:57:22 GMT
Content-Length: 119
{"results":[[JSON-DATA-ARRAY]]}
```

### Development API web interfaces
The `-dev` flag enables web interfaces to document and test the apis:
* http://localhost:8080/playground - A GraphQL playground. To use it you must have a authenticated session in the same browser.
* http://localhost:8080/swagger - A Swagger UI. To use it you have to be logged out, so no user session in the same browser. Use the JWT token with role Api generate previously to authenticate via http header.

### Use cc-backend API to start job
* Enter the URL `http://localhost:8080/swagger/index.html` in your browser.
* Enter your JWT token you generated for the API user by clicking the green Authorize button in the upper right part of the window.
* Click the `/job/start_job` endpoint and click the Try it out button.
* Enter the following json into the request body text area and fill in a recent start timestamp by executing `date +%s`.:
```
{
    "jobId":         100000,
    "arrayJobId":    0,
    "user":          "ccdemouser",
    "subCluster":    "main",
    "cluster":       "emmy",
    "startTime":    <date +%s>,
    "project":       "ccdemoproject",
    "resources":  [
        {"hostname":  "e0601"},
        {"hostname":  "e0823"},
        {"hostname":  "e0337"},
        {"hostname": "e1111"}],
    "numNodes":      4,
    "numHwthreads":  80,
    "walltime":      86400
}
```
* The response body should be the database id of the started job, for example:
```
{
  "id": 3937
}
```
* Check in ClusterCockpit
    - User `ccdemouser` should appear in Users-Tab with one running job
    - It could take up to 5 Minutes until the Job is displayed with some current data (5 Min Short-Job Filter)
    - Job then is marked with a green `running` tag
    - Metricdata displayed is read from cc-metric-store!


### Use cc-backend API to stop job
* Enter the URL `http://localhost:8080/swagger/index.html` in your browser.
* Enter your JWT token you generated for the API user by clicking the green Authorize button in the upper right part of the window.
* Click the `/job/stop_job/{id}` endpoint and click the Try it out button.
* Enter the database id at id that was returned by `start_job` and copy the following into the request body. Replace the timestamp with a recent one:
```
{
  "cluster": "emmy",
  "jobState": "completed",
  "stopTime": <RECENT TS>
}
```
* On success a json document with the job meta data is returned.

* Check in ClusterCockpit
    - User `ccdemouser` should appear in Users-Tab with one completed job
    - Job is no longer marked with a green `running` tag -> Completed!
    - Metricdata displayed is now read from job-archive!
* Check in job-archive
    - `cd ./cc-backend/var/job-archive/emmy/100/000`
    - `cd $STARTTIME`
    - Inspect `meta.json` and `data.json`

## Helper scripts
* In this tarball you can find the perl script `generate_subcluster.pl` that helps to generate the subcluster section for your system.
Usage:
* Log into an exclusive cluster node.
* The LIKWID tools likwid-topology and likwid-bench must be in the PATH!
* `$./generate_subcluster.pl` outputs the subcluster section on `stdout`

Please be aware that
* You have to enter the name and node list for the subCluster manually.
* GPU detection only works if LIKWID was build with Cuda avalable and you run likwid-topology also with Cuda loaded.
* Do not blindly trust the measured peakflops values.
* Because the script blindly relies on the CSV format output by likwid-topology this is a fragile undertaking!
