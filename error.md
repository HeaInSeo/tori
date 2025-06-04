# Issues Found

- Several source files have a trailing dash in the filename (e.g. `api/api.go-`, `v1rpc/rule_pb.go-`). Files ending with `.go-` are ignored by the Go toolchain and therefore the packages under `v1rpc` are not built or tested.
- Package `config` initializes global configuration inside `init()` which depends on an external file. Tests relying on this package may be affected by environment setup.
