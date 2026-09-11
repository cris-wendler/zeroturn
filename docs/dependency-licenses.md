# Dependency licenses

ZeroTurn has no dependencies. `go.mod` declares the module and the language version and nothing else, so there is no third party code in the executable.

| Name | Version | License | Source | Compatible with GPL-3.0-only |
| --- | --- | --- | --- | --- |
| Go standard library | the toolchain used to build | BSD-3-Clause | https://go.dev | Yes |

The standard library is linked into the executable. BSD-3-Clause is a permissive license and places no condition that conflicts with GPL-3.0-only. Go's own license file is reproduced in binaries built from it, which is satisfied by shipping the Go toolchain's notice in any distribution that includes it.

Tools used during development are not part of the released executable: Git, the Go toolchain, and, for the README recording, `jq` and `expect`.

A pull request that adds a dependency must add a row here with the license, the source, and why the compatibility holds. A dependency whose compatibility cannot be confirmed is not accepted.
