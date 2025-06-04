# Issues Found

- Previously, several source files had a trailing dash in the filename (e.g. `api/api.go-`, `v1rpc/rule_pb.go-`). These were removed so packages build and test correctly.
- Package `config` initializes global configuration inside `init()` which depends on an external file. Tests relying on this package may be affected by environment setup.
