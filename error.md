# Issues Found
- Package `config` initializes global configuration inside `init()` which depends on an external file. Tests relying on this package may be affected by environment setup.
