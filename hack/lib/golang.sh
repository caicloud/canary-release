#!/bin/bash

# options in this scripts
# GO_BUILD_PLATFORMS: Incoming variable of targets to build for.  If unset
#     then just the host architecture is built.
# GO_FASTBUILD: If set to "true", a few of architechtrues are built. .e.g linux/amd64.
# --local_build: If set, built on local. Otherwise, built in docker.

# =========================================================
# update the targets based on your project.
# =========================================================
# targets on cmd
readonly GO_BUILD_TARGETS=(
	${GO_BUILD_TARGETS[@]-}
)

# static libraries
readonly GO_STATIC_LIBRARIES=(
	${GO_STATIC_LIBRARIES[@]:-}
)

# =========================================================
# const variables
# =========================================================

# Gigabytes desired for parallel platform builds. 11 is fairly
# arbitrary, but is a reasonable splitting point for 2015
# laptops-versus-not.
readonly GO_PARALLEL_BUILD_MEMORY=4

# The golang package that we are building.
readonly GO_PACKAGE=$(util::get_go_package)

# supported minimum go version 
readonly MINIMUM_GO_VERSION="go1.8.0"

# =========================================================
# set up targets binaries platforms
# =========================================================
readonly GO_BUILD_BINARIES=("${GO_BUILD_TARGETS[@]##*/}")

if [[ -n "${GO_BUILD_PLATFORMS:-}" ]]; then
	readonly GO_BUILD_PLATFORMS=(${GO_BUILD_PLATFORMS[@]})
elif [[ "${GO_FASTBUILD:-}" == "true" ]]; then
	readonly GO_BUILD_PLATFORMS=(linux/amd64)
else
	readonly GO_BUILD_PLATFORMS=(
		linux/amd64
		darwin/amd64
	)
fi

readonly GO_ALL_BUILD_TARGETS=(
	"${GO_BUILD_TARGETS[@]-}"
)

readonly GO_ALL_BUILD_BINARIES=(
	"${GO_ALL_BUILD_TARGETS[@]##*/}"
)

# =========================================================
# functions
# =========================================================

golang::is_statically_linked_library() {
	local e
	for e in "${GO_STATIC_LIBRARIES[@]-}"; do [[ "$1" == *"/$e" ]] && return 0; done
	# Allow individual overrides--e.g., so that you can get a static build 
	# for inclusion in a container.
	if [ -n "${GO_STATIC_OVERRIDES:+x}" ]; then
		for e in "${GO_STATIC_OVERRIDES[@]}"; do [[ "$1" == *"/$e" ]] && return 0; done
	fi
	return 1
}

# golang::binaries_from_targets take a list of build targets and return the
# full go package to be built
golang::binaries_from_targets() {
	local target
	for target; do
		# If the target starts with what looks like a domain name, assume it has a
		# fully-qualified package name rather than one that needs the Kubernetes
		# package prepended.
		if [[ "${target}" =~ ^([[:alnum:]]+".")+[[:alnum:]]+"/" ]]; then
			echo "${target}"
		else
			echo "${GO_PACKAGE}/${target}"
		fi
	done
}

# Asks golang what it thinks the host platform is. The go tool chain does some
# slightly different things when the target platform matches the host platform.
golang::host_platform() {
	echo "$(go env GOHOSTOS)/$(go env GOHOSTARCH)"
}

# Takes the platform name ($1) and sets the appropriate golang env variables
# for that platform.
golang::set_platform_envs() {
	[[ -n ${1-} ]] || {
		log::error_exit "!!! Internal error. No platform set in golang::set_platform_envs"
	}

	export GOOS=${platform%/*}
	export GOARCH=${platform##*/}
	export CGO_ENABLED=${CGO_ENABLED:-}

	# Do not set CC when building natively on a platform, only if cross-compiling from linux/amd64
	if [[ $(golang::host_platform) == "linux/amd64" ]]; then
		# Dynamic CGO linking for other server architectures than linux/amd64 goes here
		# If you want to include support for more server platforms than these, add arch-specific gcc names here
		case "${platform}" in
			"linux/arm")
				export CGO_ENABLED=1
				export CC=arm-linux-gnueabihf-gcc
				;;
			"linux/arm64")
				export CGO_ENABLED=1
				export CC=aarch64-linux-gnu-gcc
				;;
			"linux/ppc64le")
				export CGO_ENABLED=1
				export CC=powerpc64le-linux-gnu-gcc
				;;
			"linux/s390x")
				export CGO_ENABLED=1
				export CC=s390x-linux-gnu-gcc
				;;
		esac
	fi
}

golang::unset_platform_envs() {
	unset GOOS
	unset GOARCH
	unset GOROOT
	unset CGO_ENABLED
	unset CC
}

# Ensure the go tool exists and is a viable version.
golang::verify_go_version() {
	if [[ -z "$(which go)" ]]; then
		log::usage_from_stdin <<EOF
Can't find 'go' in PATH, please fix and retry.
See http://golang.org/doc/install for installation instructions.
EOF
		return 2
	fi

	local go_version
	go_version=($(go version))
	local minimum_go_version
	minimum_go_version=${MINIMUM_GO_VERSION}
	if [[ "${go_version[2]}" < "${minimum_go_version}" && "${go_version[2]}" != "devel" ]]; then
		log::usage_from_stdin <<EOF
Detected go version: ${go_version[*]}.
Kubernetes requires ${minimum_go_version} or greater.
Please install ${minimum_go_version} or later.
EOF
		return 2
	fi
}

# golang::setup_env will check that the `go` commands is available in
# ${PATH}. It will also check that the Go version is good enough for the
# Kubernetes build.
#
# Inputs:
#   EXTRA_GOPATH - If set, this is included in created GOPATH
#
# Outputs:
#   env-var GOPATH points to our local output dir
#   env-var GOBIN is unset (we want binaries in a predictable place)
#   env-var GO15VENDOREXPERIMENT=1
#   current directory is within GOPATH
golang::setup_env() {
	golang::verify_go_version

	# Append EXTRA_GOPATH to the GOPATH if it is defined.
	if [[ -n ${EXTRA_GOPATH:-} ]]; then
		export GOPATH="${GOPATH}:${EXTRA_GOPATH}"
	fi

	# Assume that we are now within the GOPATH

	# Set GOROOT so binaries that parse code can work properly.
	export GOROOT=$(go env GOROOT)

	# Unset GOBIN in case it already exists in the current session.
	unset GOBIN

	# This seems to matter to some tools (godep, ginkgo...)
	export GO15VENDOREXPERIMENT=1
}

# This will take binaries from $GOPATH/bin and copy them to the appropriate
# place in ${GO_OUTPUT_BINDIR}
#
# Ideally this wouldn't be necessary and we could just set GOBIN to
# GO_OUTPUT_BINDIR but that won't work in the face of cross compilation.  'go
# install' will place binaries that match the host platform directly in $GOBIN
# while placing cross compiled binaries into `platform_arch` subdirs.  This
# complicates pretty much everything else we do around packaging and such.
golang::place_bins() {
	local host_platform
	host_platform=$(golang::host_platform)

	V=2 log::status "Placing binaries"

	local platform
	for platform in "${GO_BUILD_PLATFORMS[@]}"; do
		# The substitution on platform_src below will replace all slashes with
		# underscores.  It'll transform darwin/amd64 -> darwin_amd64.
		local platform_src="${platform//\//_}"
		if [[ $platform == $host_platform ]]; then
			# rm -f "${THIS_PLATFORM_BIN}"
			ln -sf ${PRJ_OUTPUT_BINPATH}/${platform_src}/* "${PRJ_OUTPUT_BINPATH}"
		fi

		# optional: place binaries on GOPATH/bin/GOOS/GOARCH
		# local full_binpath_src="${GOPATH}/bin/${platform_src}"
		# if [[ -d "${full_binpath_src}" ]]; then
		# 	mkdir -p "${PRJ_OUTPUT_BINPATH}/${platform}"
		# 	find "${full_binpath_src}" -maxdepth 1 -type f -exec \
		# 		rsync -pc {} "${PRJ_OUTPUT_BINPATH}/${platform}" \;
		# fi
	done
}

golang::fallback_if_stdlib_not_installable() {
	local go_root_dir=$(go env GOROOT)
	local go_host_os=$(go env GOHOSTOS)
	local go_host_arch=$(go env GOHOSTARCH)
	local cgo_pkg_dir=${go_root_dir}/pkg/${go_host_os}_${go_host_arch}_cgo

	if [ -e ${cgo_pkg_dir} ]; then
		return 0
	fi

	if [ -w ${go_root_dir}/pkg ]; then
		return 0
	fi

	log::status "+++ Warning: stdlib pkg with cgo flag not found."
	log::status "+++ Warning: stdlib pkg cannot be rebuilt since ${go_root_dir}/pkg is not writable by $(whoami)"
	log::status "+++ Warning: Make ${go_root_dir}/pkg writable for $(whoami) for a one-time stdlib install, Or"
	log::status "+++ Warning: Rebuild stdlib using the command 'CGO_ENABLED=0 go install -a -installsuffix cgo std'"
	log::status "+++ Falling back to go build, which is slower"

	local_build=true
}

# Try and replicate the native binary placement of go install without
# calling go install.
golang::output_filename_for_binary() {
	local binary=$1
	local platform=$2
	local output_path="${PRJ_OUTPUT_BINPATH}"
	# place binary in palce like project_root/bin/darwin_amd64
	output_path="${output_path}/${platform//\//_}"

	local bin=$(basename "${binary}")
	if [[ ${GOOS} == "windows" ]]; then
		bin="${bin}.exe"
	fi
	echo "${output_path}/${bin}"
}

golang::build_binaries_for_platform() {
	local platform=$1
	local local_build=${2-}

	local -a statics=()
	local -a nonstatics=()
	local -a tests=()

	V=2 log::info "Env for ${platform}: GOOS=${GOOS-} GOARCH=${GOARCH-} GOROOT=${GOROOT-} CGO_ENABLED=${CGO_ENABLED-} CC=${CC-}"

	for binary in "${binaries[@]}"; do
		if [[ "${binary}" =~ ".test"$ ]]; then
			tests+=($binary)
		elif golang::is_statically_linked_library "${binary}"; then
			statics+=($binary)
		else
			nonstatics+=($binary)
		fi
	done

	if [[ "${#statics[@]}" != 0 ]]; then
		golang::fallback_if_stdlib_not_installable
	fi

	if [[ "${local_build:-}" == "true" ]]; then
		# use local go build
		for test in "${tests[@]:+${tests[@]}}"; do
			local outfile=$(golang::output_filename_for_binary "${test}" "${platform}")
			local testpkg="$(dirname ${test})"
			go test -i -c \
				"${goflags[@]:+${goflags[@]}}" \
				-gcflags "${gogcflags}" \
				-ldflags "${goldflags}" \
				-o "${outfile}" \
				"${testpkg}"
		done

		log::progress "    "
		for binary in "${statics[@]:+${statics[@]}}"; do
			local outfile=$(golang::output_filename_for_binary "${binary}" "${platform}")
			CGO_ENABLED=0 go build -i -o "${outfile}" \
				"${goflags[@]:+${goflags[@]}}" \
				-gcflags "${gogcflags}" \
				-ldflags "${goldflags}" \
				"${binary}"
			log::progress "*"
		done
		for binary in "${nonstatics[@]:+${nonstatics[@]}}"; do
			local outfile=$(golang::output_filename_for_binary "${binary}" "${platform}")
			go build -i -o "${outfile}" \
				"${goflags[@]:+${goflags[@]}}" \
				-gcflags "${gogcflags}" \
				-ldflags "${goldflags}" \
				"${binary}"
			log::progress "*"
		done
		log::progress "\n"
	else
		# use docker build
		for test in "${tests[@]:+${tests[@]}}"; do
			local outfile=$(golang::output_filename_for_binary "${test}" "${platform}")
			local testpkg="$(dirname ${test})"
			docker run --rm \
				-v ${PRJ_ROOT}:${PRJ_ROOT} \
				-w ${PRJ_ROOT} \
				-e GOOS=${GOOS} \
				-e GOARCH=${GOARCH} \
				-e GOPATH=${GOPATH} \
				-e CGO_ENABLED=${CGO_ENABLED} \
				${GO_ONBUILD_IMAGE} \
				go test -i -c -o "${outfile}" \
				"${goflags[@]:+${goflags[@]}}" \
				-gcflags "${gogcflags}" \
				-ldflags "${goldflags}" \
				"${testpkg}"
		done

		log::progress "    "
		for binary in "${statics[@]:+${statics[@]}}"; do
			local outfile=$(golang::output_filename_for_binary "${binary}" "${platform}")
			docker run --rm \
				-v ${PRJ_ROOT}:${PRJ_ROOT} \
				-w ${PRJ_ROOT} \
				-e GOOS=${GOOS} \
				-e GOARCH=${GOARCH} \
				-e GOPATH=${GOPATH} \
				-e CGO_ENABLED=0 \
				${GO_ONBUILD_IMAGE} \
				go build -o "${outfile}" \
				"${goflags[@]:+${goflags[@]}}" \
				-gcflags "${gogcflags}" \
				-ldflags "${goldflags}" \
				"${binary}"
			log::progress "*"
		done
		for binary in "${nonstatics[@]:+${nonstatics[@]}}"; do
			local outfile=$(golang::output_filename_for_binary "${binary}" "${platform}")
			docker run --rm \
				-v ${PRJ_ROOT}:${PRJ_ROOT} \
				-w ${PRJ_ROOT} \
				-e GOOS=${GOOS} \
				-e GOARCH=${GOARCH} \
				-e GOPATH=${GOPATH} \
				-e CGO_ENABLED=${CGO_ENABLED} \
				${GO_ONBUILD_IMAGE} \
				go build -o "${outfile}" \
				"${goflags[@]:+${goflags[@]}}" \
				-gcflags "${gogcflags}" \
				-ldflags "${goldflags}" \
				"${binary}"
			log::progress "*"
		done
		log::progress "\n"
	fi

}

# Return approximate physical memory available in gigabytes.
golang::get_physmem() {
	local mem

	case "$(go env GOHOSTOS)" in
		"linux")
			# Linux kernel version >=3.14, in kb
			if mem=$(grep MemAvailable /proc/meminfo | awk '{ print $2 }'); then
				echo $((${mem} / 1048576))
				return
			fi

			# Linux, in kb
			if mem=$(grep MemTotal /proc/meminfo | awk '{ print $2 }'); then
				echo $((${mem} / 1048576))
				return
			fi
			;;
		"darwin")
			# OS X, in bytes. Note that get_physmem, as used, should only ever
			# run in a Linux container (because it's only used in the multiple
			# platform case, which is a Dockerized build), but this is provided
			# for completeness.
			if mem=$(sysctl -n hw.memsize 2>/dev/null); then
				echo $((${mem} / 1073741824))
				return
			fi
			;;
	esac

	# If we can't infer it, just give up and assume a low memory system
	echo 1
}

# Build binaries targets specified
#
# Input:
#   $@ - targets and go flags.  If no targets are set then all binaries targets
#     are built.
#   GO_BUILD_PLATFORMS - Incoming variable of targets to build for.  If unset
#     then just the host architecture is built.
golang::build_binaries() {
	# Create a sub-shell so that we don't pollute the outer environment
	(
		# Check for `go` binary and set ${GOPATH}.
		golang::setup_env
		V=2 log::info "Go version: $(go version)"

		local local_build=${LOCAL_BUILD-}

		local host_platform
		host_platform=$(golang::host_platform)

		# Use eval to preserve embedded quoted strings.
		local goflags goldflags gogcflags
		eval "goflags=(${GOFLAGS:-})"
		goldflags="${GOLDFLAGS:-} $(version::ldflags)"
		gogcflags="${GOGCFLAGS:-}"

		util::parse_args "$@"
		local -a targets=(${targets[@]-})
		goflags+=(${flags[@]-})

		if [[ ${#targets[@]} -eq 0 ]]; then
			targets=("${GO_ALL_BUILD_TARGETS[@]}")
		fi

		local -a platforms=(${GO_BUILD_PLATFORMS[@]-})
		if [[ ${#platforms[@]} -eq 0 ]]; then
			platforms=("${host_platform}")
		fi

		local binaries
		binaries=($(golang::binaries_from_targets "${targets[@]}"))

		local parallel=false
		if [[ ${#platforms[@]} -gt 1 ]]; then
			local gigs
			gigs=$(golang::get_physmem)

			if [[ ${gigs} -ge ${GO_PARALLEL_BUILD_MEMORY} ]]; then
				log::status "Multiple platforms requested and available ${gigs}G >= threshold ${GO_PARALLEL_BUILD_MEMORY}G, building platforms in parallel"
				parallel=true
			else
				log::status "Multiple platforms requested, but available ${gigs}G < threshold ${GO_PARALLEL_BUILD_MEMORY}G, building platforms in serial"
				parallel=false
			fi
		fi

		local build_on="in Docker [${GO_ONBUILD_IMAGE}]"
		if [[ ${local_build:-} == "true" ]]; then
			build_on="on localhost [${host_platform}]"
		fi

		if [[ "${parallel}" == "true" ]]; then
			log::status "Building environment:" "${build_on}"
			log::status "Building go targets for {${platforms[*]}} in parallel (output will appear in a burst when complete):" "${targets[@]}"
			local platform
			for platform in "${platforms[@]}"; do (
				golang::set_platform_envs "${platform}"
				log::status "${platform}: go build started"
				golang::build_binaries_for_platform ${platform} ${local_build:-}
				log::status "${platform}: go build finished"
			) &>"/tmp//${platform//\//_}.build" &
			done

			local fails=0
			for job in $(jobs -p); do
				wait ${job} || let "fails+=1"
			done

			for platform in "${platforms[@]}"; do
				cat "/tmp//${platform//\//_}.build"
			done

			exit ${fails}
		else
			for platform in "${platforms[@]}"; do
				log::status "Building environment:" "${build_on}"
				log::status "Building go targets for ${platform}:" "${targets[@]}"
				(
					golang::set_platform_envs "${platform}"
					golang::build_binaries_for_platform ${platform} ${local_build:-}
				)
			done
		fi
	)
}

golang::filter_tests() {
	local -a IN=($1)
	local -a exceptions=($2)
	for pkg in "${IN[@]}"; do
		local skip=
		for exception in "${exceptions[@]}"; do
			if [[ "${pkg}" =~ /${exception}(/?|$) ]]; then
				skip="true"
				break
			fi
		done

		if [[ ${skip} != "true" ]]; then
			echo "${pkg}"
		fi
	done

}

# Test all pkg except vendor, test, tests, scripts, hack
#
# Input:
#   $@ - go flags. 
#   GO_BUILD_PLATFORMS - Incoming variable of targets to build for.  If unset
#     then just the host architecture is built.
#   GO_TEST_EXCEPTIONS - Incoming variable of pkgs to test for.  If unset
#     then all pkgs are tested.
golang::unittest() {
	# Create a sub-shell so that we don't pollute the outer environment
	(
		# Check for `go` binary and set ${GOPATH}.
		golang::setup_env
		V=2 log::info "Go version: $(go version)"

		local local_build=${LOCAL_BUILD-}

		local host_platform
		host_platform=$(golang::host_platform)

		# Use eval to preserve embedded quoted strings.
		local goflags goldflags gogcflags
		eval "goflags=(${GOFLAGS:-})"
		goldflags="${GOLDFLAGS:-} $(version::ldflags)"
		gogcflags="${GOGCFLAGS:-}"

		util::parse_args "$@"
		goflags+=(${flags[@]-})

		local -a targets=($(go list ./...))
		local -a exceptions=(vendor test tests scripts hack)
		exceptions+=(${GO_TEST_EXCEPTIONS-})

		# NOTE: Using "${array[*]}" here is correct.  [@] becomes distinct words (in
		# bash parlance).
		targets=($(golang::filter_tests "${targets[*]}" "${exceptions[*]}"))
		# add .test suffix
		targets=(${targets[@]/%/\/unittest.test})

		local binaries
		binaries=($(golang::binaries_from_targets "${targets[@]}"))

		local -a platforms=(${GO_BUILD_PLATFORMS:-})
		if [[ ${#platforms[@]} -eq 0 ]]; then
			platforms=("${host_platform}")
		fi

		local build_on="in Docker [${GO_ONBUILD_IMAGE}]"
		if [[ ${local_build:-} == "true" ]]; then
			build_on="on localhost [${host_platform}]"
		fi

		for platform in "${platforms[@]}"; do
			log::status "Testing environment:" "${build_on}"
			log::status "Testing go targets for ${platform}:" "${targets[@]}"
			(
				golang::set_platform_envs "${platform}"
				golang::build_binaries_for_platform ${platform} ${local_build:-}
			)
		done
	)
}
