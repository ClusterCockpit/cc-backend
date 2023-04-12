# Release versioning

Releases are numbered with a integer id starting with 1.
Every release embeds the following assets into the binary:
* Web-frontend including javascript files and all static assets
* Golang template files for server-side rendering
* JSON schema files for validation

Remaining external assets are:
* The SQL database used
* The job archive

Both external assets are also versioned using integer ids.
This means every release binary is tied to specific versions for the SQL
database and job archive.
A command line switch `--migrate-db` is provided to migrate the SQL database
from a previous to the most recent version.
We provide a separate tool `archive-migration` to migrate an existing job
archive from the previous to the most recent version.

# Versioning of APIs
cc-backend provides two API backends:
* A REST API for querying jobs
* A GraphQL API used for data exchange between web frontend and cc-backend

Both APIs will also be versioned. We still need to decide if we also support
older REST API version using versioning of the endpoint URLs.

# How to build a specific release


# How to migrate the SQL database


# How to migrate the job archive


