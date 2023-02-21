## Docs for ClusterCockpit Searchbar

### Usage

* Searchtags are implemented as `type:<query>` search-string
    * Types `jobId, jobName, projectId, username, name` for roles `admin` and `support`
        * `jobName` is jobName as persisted in `job.meta_data` table-column
        * `username` is actual account identifier as persisted in `job.user` table-column
        * `name` is account owners name as persisted in `user.name` table-column
    * Types `jobId, jobName` for role `user`
    * Examples:
        * `jobName:myJob12`
        * `jobId:123456`
        * `username:abcd100`
        * `name:Paul`
* If no searchTag used: Best guess search with the following hierarchy
    * `jobId -> username -> name -> projectId -> jobName`
* Destinations:
    * JobId: Always Job-Table (Allows multiple identical matches, e.g. JobIds from different clusters)
    * JobName: Always Job-Table (Allows multiple identical matches, e.g. JobNames from different clusters)
    * ProjectId: Always Job-Table
    * Username
        * If *one* match found: Opens detailed user-view (`/monitoring/user/$USER`)
        * If *multiple* matches found: Opens user-table with matches listed (`/monitoring/users/`)
            * **Please Note**: Only users with jobs will be shown in table! I.e., "multiple matches" can still be only one entry in table.
    * Name
        * If *one* matching username found: Opens detailed user-view (`/monitoring/user/$USER`)
        * If *multiple* usernames found: Opens user-table with matches listed (`/monitoring/users/`)
            * **Please Note**: Only users with jobs will be shown in table! I.e., "multiple matches" can still be only one entry in table.
    * Best guess search always redirects to Job-Table or `/monitoring/user/$USER` (first username match)
* Simple HTML Error if ...
    * Best guess search fails -> 'Not Found'
    * Query `type` is unknown
    * More than two colons in string -> 'malformed'
* Spaces trimmed (both for searchTag and queryString)
    * `  job12` == `job12`
    * `projectID : abcd ` == `projectId:abcd`
* `jobName`- and `name-`queries work with a part of the target-string
    * `jobName:myjob` for jobName "myjob_cluster1"
    * `name:Paul` for name "Paul Atreides"

* JobName GQL Query is resolved as matching the query as a part of the whole metaData-JSON in the SQL DB.
