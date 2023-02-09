## Docs for ClusterCockpit Searchbar

### Usage

* Searchtags are implemented as `type:<query>` search-string
    * Types `jobId, jobName, projectId, username` for roles `admin` and `support`
    * Types `jobId, jobName` for role `user`
    * Examples:
        * `jobName:myJob12`
        * `jobId:123456`
        * `username:abcd100`
* If no searchTag used: Best guess search with the following hierarchy
    * `jobId -> username -> projectId -> jobName`
* Simple HTML Error if ...
    * Best guess search fails -> 'Not Found'
    * Query `type` is unknown
    * More than two colons in string -> 'malformed'
* Spaces trimmed (both for searchTag and queryString)
    * `  job12` == `job12`
    * `projectID : abcd ` == `projectId:abcd`
* jobId-Query now redirects to table
    * Allows multiple jobs from different systems, but with identical job-id to be found
* jobName-Query works with a part of the jobName-String (e.g. jobName:myjob for jobName myjob_cluster1)
    * JobName GQL Query is resolved as matching the query as a part of the whole metaData-JSON in the SQL DB.
