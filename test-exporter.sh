#!/bin/sh
# Copyright 2022 Intel Corporation
# SPDX-License-Identifier: Apache-2.0
#
# Script for testing that querying fakedev-exporter works,
# and that it blocks undesired queries

set -e

LINE="-----------------------------"
TEST_ADDR="127.0.0.1:9999"
TEST_URL="http://$TEST_ADDR/metrics"
SOCKET="/tmp/fakedev-exporter.socket"
DEVICES="card0,card1"
pid=0

error_exit () {
	if [ $pid -gt 0 ]; then
		kill $pid
	fi
	echo "Test script for 'fakedev-exporter'."
	echo
	echo "Usage: ${0##*/} [fakedev-exporter [fakedev-workload [invalid-workload]]]"
	echo
	echo "Paths to given fakedev-* binaries need to be absolute."
	echo
	echo "ERROR: $1!"
	exit 1
}

if [ -z "$(which wget)" ]; then
	error_exit "'wget' tool missing"
fi
if [ -z "$(which diff)" ]; then
	error_exit "'diff' tool missing"
fi

if [ ! -w . ]; then
	error_exit "current directory is not writable by tests!"
fi

FAKEDEV="$PWD/fakedev-exporter"
if [ $# -gt 0 ]; then
	FAKEDEV="$1"
	shift
fi
WORKLOAD="$PWD/fakedev-workload"
if [ $# -gt 0 ]; then
	WORKLOAD="$1"
	shift
fi
INVALID="$PWD/invalid-workload"
if [ $# -gt 0 ]; then
	INVALID="$1"
	shift
fi
if [ ! -x "$FAKEDEV" ]; then
	error_exit "'$FAKEDEV' (fakedev-exporter) missing, or not executable"
fi
if [ ! -x "$WORKLOAD" ]; then
	error_exit "'$WORKLOAD' (fakedev-workload) missing, or not executable"
fi
if [ ! -x "$INVALID" ]; then
	error_exit "'$INVALID' (invalid-workload) missing, or not executable"
fi

if ! cd "${0%/*}/configs"; then
	error_exit "fakedev-exporter 'configs' dir missing"
fi

echo "Run ${FAKEDEV##*/} (on background)..."
"$FAKEDEV" \
	--count 2 \
	--socket $SOCKET \
	--address $TEST_ADDR \
	--devlist devices/devlist.json \
	--devtype devices/dg1-4905.json \
	--identity identity/xpu-manager.json \
	--wl-all workloads/load-10-exact.json \
	& # no args
pid=$!
sleep 1

# back to work dir
if ! cd -; then
	error_exit "return back to work dir failed"
fi

export http_proxy=

echo "$LINE"
echo "*** Check that server does not accept invalid WL specs ***"
if ! "$INVALID" -devnames "$DEVICES" -socket $SOCKET --url $TEST_URL; then
	error_exit "communication failure, or server accepted invalid WL spec"
fi

echo "$LINE"
echo "*** Test parallel queries ***"
MAX=16
for i in $(seq 2 $MAX); do
	if [ "$((i%2))" -eq 0 ]; then
		# add workloads (adding 0% load) while exporter is being queried
		"$WORKLOAD" -socket $SOCKET -name Dummy -activity 0:0:0 -repeat 0 -devnames "$DEVICES" &
	fi
	wget -b -O"metrics$i" -q $TEST_URL
done
if ! killall "${WORKLOAD##*/}"; then
	error_exit "workload killing failed"
fi
wget -b -O"metrics1" -q $TEST_URL
sleep 1
errors=0
for i in $(seq 2 $MAX); do
	if ! cmp "metrics1" "metrics$i"; then
		echo "WARN: metric data from fetch $i differs from first one ('metrics1' != 'metrics$i')"
		diff -u "metrics1" "metrics$i"
		errors=$((errors+1))
	fi
	rm "metrics$i"
done
rm "metrics1"
if [ $errors -gt 0 ]; then
	error_exit "mismatch(es) in parallel fetches"
else
	echo "=> results matched from $MAX queries (done while $((MAX/2)) WLs were added)"
fi

check_fetch () {
	echo "try: wget -O- --no-verbose $*"
	wget -O- --no-verbose "$@"
}

echo "$LINE"
echo "*** Test normal (GET) query method working ***"
if ! check_fetch "$TEST_URL"; then
	error_exit "metric fetch failed"
fi

echo "$LINE"
echo "*** Test longer URL query being blocked ***"
if check_fetch "$TEST_URL/foobar"; then
	error_exit "longer URL accepted"
fi

echo "$LINE"
echo "*** Test HEAD method being blocked ***"
if check_fetch --method HEAD "$TEST_URL"; then
	error_exit "HEAD method accepted"
fi

echo "$LINE"
echo "*** Test DELETE method being blocked ***"
if check_fetch --method DELETE "$TEST_URL"; then
	error_exit "DELETE method accepted"
fi

echo "$LINE"
echo "*** Test invalid (FOOBAR) method being blocked ***"
if check_fetch --method FOOBAR "$TEST_URL"; then
	error_exit "FOOBAR method accepted"
fi

echo "$LINE"
echo "*** Test POST/BODY query being blocked ***"
if check_fetch --post-data="user=test" "$TEST_URL"; then
	error_exit "POST method / BODY content accepted"
fi

echo "$LINE"
echo "Terminating '$FAKEDEV'..."
if ! kill $pid; then
	error_exit "killing fakedev-exporter failed"
fi
pid=0

wait %1
ret=$?
if [ $ret -ne 0 ]; then
	error_exit "fakedev-exporter terminated with exit code $ret"
fi

echo "$LINE"
echo "=> SUCCESS!"
