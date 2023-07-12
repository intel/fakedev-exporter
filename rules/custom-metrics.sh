#!/bin/sh
# Copyright 2022 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

# args: error message
usage ()
{
	name=${0##*/}
	cat << EOF

Outputs header/rules/footer YAML files from dir containing the given
metric rule file(s), with rule files content repeated for each given
CID/CARD/BDF spec argument, and following strings in rules files:
* "CID" (device file name index),
* "CARD" (define file name, acting as GPU plugin GPU ID), and
* "BDF" (device PCI address)
being replaced using values from the CID/CARD/BDF argument(s).

Usage:
	$name  <rule file(s)>  <CID/CARD/BDF spec(s)>

Example:
	$name  *-rules.yaml  0/card0/03:00.0 1/card1/0a:00.0

ERROR: $1!
EOF
	exit 1
}

if [ $# -lt 1 ]; then
	usage "HELP"
fi

rules=""
# check which args exist as files, and take those as rule files
while true; do
	if [ $# -lt 1 ]; then
		break
	fi
	if [ ! -f "$1" ]; then
		break
	fi
	dir=${1%/*}
	if [ "$dir" = "$1" ]; then
		dir="."
	fi
	rules="$rules $1"
	shift
done

if [ $# -lt 1 ]; then
	usage "no <CID/CARD/BDF> arguments given"
fi
if [ -z "$rules" ]; then
	usage "'$1' not found / no rule files given"
fi

header=$dir/header.yaml
footer=$dir/footer.yaml
if [ ! -f "$header" ] || [ ! -f "$footer" ]; then
	usage "$header / $footer missing"
fi

for spec in "$@"; do
	items=$(echo "$spec" | tr '/' ' ' | wc -w)
	if [ "$items" -lt 3 ]; then
		usage "'$spec' spec missing component(s)"
	fi
	if [ "$items" -gt 3 ]; then
		usage "too many components in '$spec' spec"
	fi
done

echo "# GENERATOR: ${0##*/}"
echo "# RULE FILE(s):$rules"
echo "# CID/CARD/BDF: $*"

cat "$header"
for spec in "$@"; do
	cid=${spec%%/*}
	card=${spec%/*}
	card=${card#*/}
	bdf=${spec##*/}
	# shellcheck disable=SC2086
	sed -e "s/CID/$cid/g" -e "s/CARD/$card/g" -e "s/BDF/$bdf/g" $rules
done
cat "$footer"
