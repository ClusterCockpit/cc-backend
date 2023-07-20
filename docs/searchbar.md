# Docs for ClusterCockpit Searchbar

## Usage

* Searchtags are implemented as `type:<query>` search-string
  * Types `jobId, jobName, projectId, username, name, arrayJobId` for roles `admin` and `support`
    * `jobName` is jobName as persisted in `job.meta_data` table-column
    * `username` is actual account identifier as persisted in `job.user` table-column
    * `name` is account owners name as persisted in `user.name` table-column
  * Types `jobId, jobName, projectId, arrayJobId` for role `user`
  * Examples:
    * `jobName:myJob12`
    * `jobId:123456`
    * `username:abcd100`
    * `name:Paul`
* If no searchTag used: Best guess search with the following hierarchy
  * `jobId -> username -> name -> projectId -> jobName`
* Destinations:
  * JobId: Job-Table (Allows multiple identical matches, e.g. JobIds from different clusters)
  * JobName: Job-Table (Allows multiple identical matches, e.g. JobNames from different clusters)
  * ProjectId: Job-Table
  * Username: Users-Table
    * **Please Note**: Only users with jobs will be shown in table! I.e., Users without jobs will be missing in table. Also, a `Last 30 Days` is active by default and might filter out expected users.
  * Name: Users-Table
    * **Please Note**: Only users with jobs will be shown in table! I.e., Users without jobs will be missing in table. Also, a `Last 30 Days` is active by default and might filter out expected users.
  * ArrayJobId: Job-Table (Lists all Jobs of Queried ArrayJobId)
  * Best guess search always redirects to Job-Table or `/monitoring/user/$USER` (first username match)
  * Unprocessable queries will display messages detailing the cause (Info, Warning, Error)
* Spaces trimmed (both for searchTag and queryString)
  * `  job12` == `job12`
  * `projectID : abcd ` == `projectId:abcd`
* `jobName`- and `name-`queries work with a part of the target-string
  * `jobName:myjob` for jobName "myjob_cluster1"
  * `name:Paul` for name "Paul Atreides"

* JobName GQL Query is resolved as matching the query as a part of the whole metaData-JSON in the SQL DB.
