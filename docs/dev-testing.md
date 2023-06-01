## Overview

We use the standard golang testing environment.

The following conventions are used:

* *White box unit tests*: Tests for internal functionality are placed in files
* *Black box unit tests*: Tests for public interfaces are placed in files
with `<package name>_test.go` and belong to the package `<package_name>_test`.
There only exists one package test file per package.
* *Integration tests*: Tests that use multiple componenents are placed in a
package test file. These are named `<package name>_test.go` and belong to the
package `<package_name>_test`.
* *Test assets*: Any required files are placed in a directory `./testdata`
within each package directory.

## Executing tests

Visual Studio Code has a very good golang test integration.
For debugging a test this is the recommended solution.

The Makefile provided by us has a `test` target that executes:
```
$ go clean -testcache
$ go build ./...
$ go vet ./...
$ go test ./...
```

Of course the commands can also be used on the command line.
For details about golang testing refer to the standard documentation:

* [Testing package](https://pkg.go.dev/testing)
* [go test command](https://pkg.go.dev/cmd/go#hdr-Test_packages)
