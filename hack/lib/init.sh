#!/bin/bash

# Exit on error. Append "|| true" if you expect an error.
set -o errexit
# Do not allow use of undefined vars. Use ${VAR:-} to use an undefined VAR
set -o nounset
# Catch the error in case mysqldump fails (but gzip succeeds) in `mysqldump |gzip`
set -o pipefail

# The root of the build/dist directory
PRJ_ROOT="$(cd "$(dirname "${BASH_SOURCE}")/../.." && pwd -P)"
PRJ_CMDPATH="${PRJ_ROOT}/cmd"
PRJ_OUTPUT_BINPATH="${PRJ_ROOT}/bin"

GO_ONBUILD_IMAGE="${GO_ONBUILD_IMAGE:-golang:1.9.2-alpine3.6}"
COLOR_LOG=true

source "${PRJ_ROOT}/hack/lib/util.sh"
source "${PRJ_ROOT}/hack/lib/logging.sh"

log::install_errexit

source "${PRJ_ROOT}/hack/lib/version.sh"
source "${PRJ_ROOT}/hack/lib/golang.sh"
source "${PRJ_ROOT}/hack/lib/docker.sh"

PRJ_OUTPUT_HOSTBIN="${PRJ_OUTPUT_BINPATH}/$(util::host_platform)"
