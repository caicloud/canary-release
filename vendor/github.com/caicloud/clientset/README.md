# clientset
A set of kubernetes api client for all native resources and tprs

Usage:

1. Defines types in `pkg/apis/`
2. Add packages to `./cmd/autogenerate.sh`($PKGS)
3. Run command: `./cmd/autogenerate.sh` to generate client