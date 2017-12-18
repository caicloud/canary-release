#!/bin/bash

# -----------------------------------------------------------------------------
# Version management helpers.  These functions help to set, save and load the
# following variables:
#
#    PRJ_GIT_COMMIT - The git commit id corresponding to this
#          source code.
#    PRJ_GIT_TREE_STATE - "clean" indicates no changes since the git commit id
#        "dirty" indicates source code changes after the git commit id
#        "archive" indicates the tree was produced by 'git archive'
#    PRJ_GIT_VERSION - "vX.Y" used to indicate the last release version.
#    PRJ_GIT_REMOTE - The git remote origin url.

# Grovels through git to set a set of env variables.
#
# If PRJ_GIT_VERSION_FILE, this function will load from that file instead of
# querying git.
version::get_version_vars() {
	if [[ -n ${PRJ_GIT_VERSION_FILE-} ]]; then
		version::load_version_vars "${PRJ_GIT_VERSION_FILE}"
		return
	fi

	# If the project source was exported through git archive, then
	# we likely don't have a git tree, but these magic values may be filled in.
	if [[ '$Format:%%$' == "%" ]]; then
		PRJ_GIT_COMMIT='$Format:%H$'
		PRJ_GIT_TREE_STATE="archive"
		# When a 'git archive' is exported, the '$Format:%D$' below will look
		# something like 'HEAD -> release-1.8, tag: v1.8.3' where then 'tag: '
		# can be extracted from it.
		if [[ '$Format:%D$' =~ "tag:\ (v[^ ]+)" ]]; then
			PRJ_GIT_VERSION="${BASH_REMATCH[1]}"
		fi
	fi

	local git=(git --work-tree "${PRJ_ROOT}")

	if [[ -z ${PRJ_GIT_REMOTE-} ]]; then
		PRJ_GIT_REMOTE="$("${git[@]}" remote get-url origin 2>/dev/null)"
	fi

	if [[ -n ${PRJ_GIT_COMMIT-} ]] || PRJ_GIT_COMMIT=$("${git[@]}" rev-parse "HEAD^{commit}" 2>/dev/null); then
		if [[ -z ${PRJ_GIT_TREE_STATE-} ]]; then
			# Check if the tree is dirty.  default to dirty
			if git_status=$("${git[@]}" status --porcelain 2>/dev/null) && [[ -z ${git_status} ]]; then
				PRJ_GIT_TREE_STATE="clean"
			else
				PRJ_GIT_TREE_STATE="dirty"
			fi
		fi

		# Use git describe to find the version based on annotated tags.
		if [[ -n ${PRJ_GIT_VERSION-} ]] || PRJ_GIT_VERSION=$("${git[@]}" describe --tags --abbrev=14 "${PRJ_GIT_COMMIT}^{commit}" 2>/dev/null); then
			# '+' can not be used in docker tag
			PRJ_DOCKER_TAG=${PRJ_GIT_VERSION}
			# This translates the "git describe" to an actual semver.org
			# compatible semantic version that looks something like this:
			#   v1.1.0-alpha.0.6+84c76d1142ea4d
			#
			# TODO: We continue calling this "git version" because so many
			# downstream consumers are expecting it there.
			DASHES_IN_VERSION=$(echo "${PRJ_GIT_VERSION}" | sed "s/[^-]//g")
			if [[ "${DASHES_IN_VERSION}" == "---" ]]; then
				# We have distance to subversion (v1.1.0-subversion-1-gCommitHash)
				PRJ_GIT_VERSION=$(echo "${PRJ_GIT_VERSION}" | sed "s/-\([0-9]\{1,\}\)-g\([0-9a-f]\{14\}\)$/.\1\+\2/")
				PRJ_DOCKER_TAG=$(echo "${PRJ_DOCKER_TAG}" | sed "s/-\([0-9]\{1,\}\)-g\([0-9a-f]\{14\}\)$/.\1\-\2/")
			elif [[ "${DASHES_IN_VERSION}" == "--" ]]; then
				# We have distance to base tag (v1.1.0-1-gCommitHash)
				PRJ_GIT_VERSION=$(echo "${PRJ_GIT_VERSION}" | sed "s/-g\([0-9a-f]\{14\}\)$/+\1/")
				PRJ_DOCKER_TAG=$(echo "${PRJ_DOCKER_TAG}" | sed "s/-g\([0-9a-f]\{14\}\)$/-\1/")
			fi
			if [[ "${PRJ_GIT_TREE_STATE}" == "dirty" ]]; then
				# git describe --dirty only considers changes to existing files, but
				# that is problematic since new untracked .go files affect the build,
				# so use our idea of "dirty" from git status instead.
				PRJ_GIT_VERSION+="-dirty"
				PRJ_DOCKER_TAG+="-dirty"
			fi
		fi

		# no tags found
		if [[ -z ${PRJ_GIT_VERSION-} ]]; then
			PRJ_GIT_VERSION="v0.0.0"
			PRJ_DOCKER_TAG="v0.0.0"
			if [[ "${PRJ_GIT_TREE_STATE}" == "dirty" ]]; then
				PRJ_GIT_VERSION+="-dirty"
				PRJ_DOCKER_TAG+="-dirty"
			fi
		fi
	fi
}

# Saves the environment flags to $1
version::save_version_vars() {
	local version_file=${1-}
	[[ -n ${version_file} ]] || {
		echo "!!! Internal error.  No file specified in version::save_version_vars"
		return 1
	}

	cat <<EOF >"${version_file}"
PRJ_GIT_COMMIT='${PRJ_GIT_COMMIT-}'
PRJ_GIT_TREE_STATE='${PRJ_GIT_TREE_STATE-}'
PRJ_GIT_VERSION='${PRJ_GIT_VERSION-}'
PRJ_GIT_REMOTE='${PRJ_GIT_REMOTE}'
EOF
}

# Loads up the version variables from file $1
version::load_version_vars() {
	local version_file=${1-}
	[[ -n ${version_file} ]] || {
		echo "!!! Internal error.  No file specified in version::load_version_vars"
		return 1
	}

	source "${version_file}"
}

version::ldflag() {
	local key=${1}
	local val=${2}

	# If you update these, also update the list pkg/version/def.bzl.
	echo "-X ${GO_PACKAGE}/pkg/version.${key}=${val}"
}

# Prints the value that needs to be passed to the -ldflags parameter of go build
# in order to set the project based on the git tree status.
# IMPORTANT: if you update any of these, also update the lists in
# pkg/version/def.bzl and hack/print-workspace-status.sh.
version::ldflags() {
	version::get_version_vars
	local buildDate=
	[[ -z ${SOURCE_DATE_EPOCH-} ]] || buildDate="--date=@${SOURCE_DATE_EPOCH}"
	local -a ldflags=($(version::ldflag "buildDate" "$(date ${buildDate} -u +'%Y-%m-%dT%H:%M:%SZ')"))
	if [[ -n ${PRJ_GIT_COMMIT-} ]]; then
		ldflags+=($(version::ldflag "gitCommit" "${PRJ_GIT_COMMIT}"))
		ldflags+=($(version::ldflag "gitTreeState" "${PRJ_GIT_TREE_STATE}"))
	fi

	if [[ -n ${PRJ_GIT_VERSION-} ]]; then
		ldflags+=($(version::ldflag "version" "${PRJ_GIT_VERSION}"))
	fi

	if [[ -n ${PRJ_GIT_REMOTE-} ]]; then
		ldflags+=($(version::ldflag "gitRemote" "${PRJ_GIT_REMOTE}"))
	fi

	# The -ldflags parameter takes a single string, so join the output.
	echo "${ldflags[*]-}"
}
