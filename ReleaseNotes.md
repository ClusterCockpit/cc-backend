# `cc-backend` version 1.2.0

Supports job archive version 1 and database version 6.

This is a minor release of `cc-backend`, the API backend and frontend
implementation of ClusterCockpit.

** Breaking changes **

* The LDAP configuration option user_filter was changed and now should not include
the uid wildcard. Example:
   - Old: `"user_filter": "(&(objectclass=posixAccount)(uid=*))"`
   - New: `"user_filter": "(&(objectclass=posixAccount))"`

* The aggregate job statistic core hours is now computed using the job table
column `num_hwthreads`. In a future release this column will be renamed to
`num_cores`. For correct display of core hours `num_hwthreads` must be correctly
filled on job start. If your existing jobs do not provide the correct value in
this column then you can set this with one SQL INSERT statement. This only applies
if you have exclusive jobs, only. Please be aware that we treat this column as
it is the number of cores. In case you have SMT enabled and `num_hwthreads`
is not the number of cores the core hours will be too high by a factor!

* The jwts key is now mandatory in config.json. It has to set max-age for
  validity. Some key names have changed, please refer to
  [config documentation](./configs/README.md) for details.

** NOTE **
If you are using the sqlite3 backend the `PRAGMA` option `foreign_keys` must be
explicitly set to ON. If using the sqlite3 console it is per default set to
OFF!  On every console session you must set:
```
sqlite> PRAGMA foreign_keys = ON;

```
Otherwise if you delete jobs the jobtag relation table will not be updated accordingly!
