#!/bin/sh
# Copyright 2022 Intel Corporation
# SPDX-License-Identifier: Apache-2.0
#
# Script for checking fakedev-exporter deployment files security settings

LINE="-----------------------------"

error_exit () {
	echo
	echo "ERROR: $1!"
	exit 1
}

echo "$LINE"
echo "*** Check deployment security: ***"
echo
if ! cd "${0%/*}/deployments"; then
	error_exit "fakedev-exporter 'deployments' dir missing"
fi

# pod/container security context items
user="^ *runAsUser *:"
capadd="^ *add *:"
capdrop="^ *drop *:"
seccomp="^ *seccompProfile *:"
prof_ok="^ .* type *: *RuntimeDefault"
prof_fail="^ .* type *: *Unconfined"
readonly="^ *readOnlyRootFilesystem *:"
escalation="^ *allowPrivilegeEscalation *:"
for yaml in fakedev-exporter.yaml workloads/*.yaml; do
	echo "$yaml:"
	if [ ! -f "$yaml" ]; then
		error_exit "'$yaml' missing"
	fi
	grep "$user" "$yaml"
	if ! grep "$user" "$yaml" | grep -v -q root; then
		error_exit "'$yaml' deployment uses 'root' user"
	fi
	grep "$capadd" "$yaml"
	if grep -q "$capadd" "$yaml"; then
		error_exit "'$yaml' deployment adds capabilities"
	fi
	grep "$capdrop" "$yaml"
	if ! grep "$capdrop" "$yaml" | grep -q '"ALL"'; then
		error_exit "'$yaml' deployment does not drop all capabilities"
	fi
	if ! grep "$seccomp" "$yaml" || grep "$prof_fail" "$yaml" || ! grep "$prof_ok" "$yaml"; then
		error_exit "'$yaml' deployment lacks seccomp restrictions"
	fi
	grep "$readonly" "$yaml"
	if ! grep "$readonly" "$yaml" | grep -q true; then
		error_exit "'$yaml' deployment rootfs not set readonly"
	fi
	grep "$escalation" "$yaml"
	if ! grep "$escalation" "$yaml" | grep -q false; then
		error_exit "'$yaml' deployment privilege escalation allowed"
	fi
	echo "$LINE"
done

echo "=> SUCCESS!"
