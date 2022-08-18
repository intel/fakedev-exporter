// Copyright 2022 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"log"
	"os"
	"sort"
)

// limitT state min and max values for given metr, which could act also
// as its default value
type limitT struct {
	Min float64
	Max float64
}

type devtypeT struct {
	// labels for all devices
	DeviceLabels map[string]string
	// per-metric limits
	MetricLimits map[string]limitT
}

type devlistT struct {
	// per-device labels
	DeviceLabels []map[string]string
}

type labelPairT struct {
	name, value string
}

// devinfoT stores both common and per-device information on device labels,
// what are their metric limits, metric specific labels and which of those
// should be exported.  Latter two are filled based on identity info
type devinfoT struct {
	// per-device labels (len=device count)
	deviceLabels [][]labelPairT
	// per-metric limits
	metricLimits map[string]limitT

	// per-device file name -> array index mapping
	devicemap map[string]int
	// per-metric labels (if any)
	metricLabels map[string][]labelPairT
	// list of metrics to output
	output []string
}

// identityT maps devinfo metric and label names to exporter ones.
// If name is missing, it's not output. If value is "", name is not changed.
// NOTE: member names need to be capitalized for JSON marshaling to use them.
type identityT struct {
	DeviceLabelMap map[string]string
	MetricMap      map[string]string
	MetricLabels   map[string]map[string]string
}

// mapLabels removes labels from mapping which do not exist in exporter identity,
// and maps rest to identity label names. Returns the (label,value) slice as result,
// and list of labels missing a mapping
func mapLabels(labels map[string]string, identity identityT) ([]labelPairT, []string) {
	missing := make([]string, 0)
	result := make([]labelPairT, 0)
	for label, value := range labels {
		name, exists := identity.DeviceLabelMap[label]
		if exists {
			// log.Printf("device label mapping: '%s' -> '%s' = '%s'", label, name, value)
			result = append(result, labelPairT{name, value})
		} else {
			// log.Printf("missing mapping for device label '%s'", label)
			missing = append(missing, label)
		}
	}
	return result, missing
}

// sortLabelList sorts given references label list and return its reference
func sortLabelList(labels []labelPairT) []labelPairT {
	sort.SliceStable(labels, func(i, j int) bool { return labels[i].name < labels[j].name })
	return labels
}

// getDevinfo is called at startup to load device information from specified
// JSON config files.  devcount specifies how many devices (with per-instance labels
// from devlist) are to be created.  Device label and limit names are mapped based on
// loaded exporter identity. Labels are also filtered by identity at startup, but
// metrics only on output.  Latter is to make sure that metric derivation works correctly.
func getDevinfo(devcount int, typefile, listfile, idfile string) devinfoT {
	var (
		identity  identityT
		devtype   devtypeT
		devlist   devlistT
		devlabels []labelPairT
		info      devinfoT
		text      []byte
		err       error
	)
	if text, err = os.ReadFile(idfile); err != nil {
		log.Fatalf("Unable to read exporter identity JSON file '%s': %v", idfile, err)
	}
	log.Printf("identity: %s\n", string(text))
	if err = json.Unmarshal(text, &identity); err != nil {
		log.Fatalf("Unmarshaling failed for identity JSON file '%s': %v", idfile, err)
	}

	if text, err = os.ReadFile(typefile); err != nil {
		log.Fatalf("Unable to read device type JSON file '%s': %v", typefile, err)
	}
	log.Printf("devtype: %s\n", string(text))
	if err = json.Unmarshal(text, &devtype); err != nil {
		log.Fatalf("Unmarshaling failed for device type JSON file '%s': %v", typefile, err)
	}
	var missing []string
	devlabels, missing = mapLabels(devtype.DeviceLabels, identity)
	if len(missing) > 0 {
		log.Printf("WARN: no identity mapping for device type labels: %v", missing)
	}

	if text, err = os.ReadFile(listfile); err != nil {
		log.Fatalf("Unable to read device list JSON file '%s': %v", listfile, err)
	}
	if err = json.Unmarshal(text, &devlist); err != nil {
		log.Fatalf("Unmarshaling failed for device list JSON file '%s': %v", listfile, err)
	}
	if len(devlist.DeviceLabels) < devcount {
		log.Fatalf("Device list contains fewer devices than requested (%d < %d): %s",
			len(devlist.DeviceLabels), devcount, listfile)
	}
	var (
		name   string
		exists bool
	)
	info.deviceLabels = make([][]labelPairT, devcount)
	info.devicemap = make(map[string]int, devcount)
	for dev := 0; dev < devcount; dev++ {
		if _, exists := devlist.DeviceLabels[dev]["file"]; !exists {
			log.Fatalf("devlist[%d] missing 'file' label (used for matching WL device file names)", dev)
		}
		info.devicemap[devlist.DeviceLabels[dev]["file"]] = dev

		// map per-device labels + add (already mapped) type labels
		labels, missing := mapLabels(devlist.DeviceLabels[dev], identity)
		if len(missing) > 0 {
			log.Printf("WARN: no identity mapping for devlist[%d] labels: %v", dev, missing)
		}
		for _, label := range devlabels {
			labels = append(labels, label)
		}
		// warn of missing labels before assignment
		for label, value := range identity.DeviceLabelMap {
			if _, exists := devlist.DeviceLabels[dev][value]; exists {
				continue
			}
			if _, exists := devtype.DeviceLabels[value]; exists {
				continue
			}
			log.Printf("WARN: device[%d] label missing for identity mapping: '%s'", dev, label)
		}
		info.deviceLabels[dev] = sortLabelList(labels)
	}
	// complain about missing metrics
	for metric := range identity.MetricMap {
		if _, exists := devtype.MetricLimits[metric]; !exists {
			log.Printf("WARN: no device type metric/limit for identity mapping: '%s'", metric)
		}
	}
	// map (matching) device metric limit names to output names
	info.metricLimits = make(map[string]limitT)
	for metric, limit := range devtype.MetricLimits {
		if name, exists = identity.MetricMap[metric]; !exists {
			log.Printf("WARN: no identity mapping for device metric/limit: '%s'", metric)
			continue
		}
		info.metricLimits[name] = limit
		log.Printf("metric/limit name identity mapping: '%s' -> '%s'", metric, name)
	}
	// map metric info labels
	info.metricLabels = make(map[string][]labelPairT, len(identity.MetricLabels))
	for metric, labels := range identity.MetricLabels {
		name, exists := identity.MetricMap[metric]
		if !exists {
			log.Fatalf("identity MetricMap[%s] missing for MetricLabels", metric)
		}
		i := 0
		ll := make([]labelPairT, len(labels))
		for label, value := range labels {
			ll[i] = labelPairT{label, value}
			i++
		}
		info.metricLabels[name] = sortLabelList(ll)
	}
	// which device metrics to output
	i := 0
	out := make([]string, len(identity.MetricMap))
	for _, name := range identity.MetricMap {
		out[i] = name
		i++
	}
	sort.Strings(out)
	info.output = out
	return info
}
