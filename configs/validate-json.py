#!/usr/bin/python3
# Copyright 2024 Intel Corporation
# SPDX-License-Identifier: Apache-2.0
"validate JSON files in subdirs"

import glob
import json

for name in glob.glob("*/*.json"):
    print("-", name)
    try:
        json.load(open(name, encoding="utf-8"))
    except json.decoder.JSONDecodeError as e:
        print("  ERROR:", e)
