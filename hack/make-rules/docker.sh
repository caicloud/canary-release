#!/bin/bash

# Exit on error. Append "|| true" if you expect an error.
set -o errexit
# Do not allow use of undefined vars. Use ${VAR:-} to use an undefined VAR
set -o nounset
# Catch the error in case mysqldump fails (but gzip succeeds) in `mysqldump |gzip`
set -o pipefail

PRJ_ROOT=$(dirname "${BASH_SOURCE}")/../..
VERBOSE="${VERBOSE:-1}"
source "${PRJ_ROOT}/hack/lib/init.sh"

if [[ -n ${PRJ_DOCKER_BUILD-} ]]; then
	docker::build_images "$@"
fi

if [[ -n ${PRJ_DOCKER_PUSH-} ]]; then
	docker::push_images "$@"
fi
