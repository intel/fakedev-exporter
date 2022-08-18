// Copyright 2022 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// wlProfileT values are in percents and seconds
type profileT struct {
	Load        uint
	Fluctuation uint
	Seconds     uint
}

type workloadT struct {
	Name    string
	Repeat  uint
	Profile []profileT
	Devices []string
	Limits  map[string]float64
}

// getDevices() returns list of device (base) file names matching
// the device glob pattern. If none are found, process terminates
func getDevices(glob string) []string {
	// available device file name paths
	paths, err := filepath.Glob(glob)
	if paths == nil || err != nil {
		log.Fatalf("ERROR: no files matching glob pattern '%s'", glob)
	}
	devices := make([]string, len(paths))
	for i, dev := range paths {
		devices[i] = path.Base(dev)
	}
	return devices
}

// getDevnames() replaces "INDEX" with $JOB_COMPLETION_INDEX % max value in given string,
// if max is set. Returns results split at commas
func getDevnames(names string, max int) []string {
	if max > 0 {
		// https://kubernetes.io/docs/tasks/job/indexed-parallel-processing-static/
		jobi := os.Getenv("JOB_COMPLETION_INDEX")
		if jobi == "" {
			log.Fatalf("ERROR: max-index option used, but $JOB_COMPLETION_INDEX not set")
		}
		if index, err := strconv.Atoi(jobi); err != nil {
			log.Fatalf("ERROR: parsing $JOB_COMPLETION_INDEX value ('%s') failed: %v", jobi, err)
		} else {
			names = strings.ReplaceAll(names, "INDEX", strconv.Itoa(index%max))
		}
	}
	return strings.Split(names, ",")
}

// parseProfiles() parses given activity profile list spec, if one is given,
// and overrides the WL list with it.  Terminates if list is empty.
func parseProfiles(spec string) []profileT {
	if spec == "" {
		return []profileT{}
	}
	profile := make([]profileT, 0)
	for _, act := range strings.Split(spec, ",") {
		var load, flux, secs uint
		n, err := fmt.Sscanf(act, "%v:%v:%v", &load, &flux, &secs)
		if err != nil || n != 3 {
			log.Fatalf("ERROR: profile '%s' - %d integers, not 3: %v", act, n, err)
		}
		if load > 100 || flux > load {
			log.Fatalf("ERROR: invalid load (%d > 100) or fluctuation (%d > load) values in '%s'", load, flux, act)
		}
		profile = append(profile, profileT{load, flux, secs})
	}
	return profile
}

func parseJSON(name string, wl *workloadT) {
	if name == "" {
		return
	}
	if data, err := os.ReadFile(name); err == nil {
		if err = json.Unmarshal(data, &wl); err != nil {
			log.Fatalf("ERROR: Unmarshaling JSON spec file '%s' failed", name)
		}
	}
}

func parseArgs() (workloadT, string) {
	wl := workloadT{}
	var activity, devices, devnames, json, socket string
	flag.StringVar(&wl.Name, "name", "Workload", "Workload / pod name")
	flag.UintVar(&wl.Repeat, "repeat", 1, "How many times activity is simulated, 0 = forever")
	flag.StringVar(&activity, "activity", "98:1:0", "Comma separated list of '<load>:<fluctuation>:<seconds>' device utilization percentage and duration")
	flag.StringVar(&devices, "devices", "/dev/dri/card*", "Glob pattern for matching device file(s) (mapped to WL container) on which activity is to be simulated")
	flag.StringVar(&devnames, "devnames", "", "Instead of matching devices assigned by device plugin, simulate activity on given comma separate list of device(s)")
	flag.StringVar(&socket, "socket", "/tmp/fakedev-exporter", "Unix socket for workload communication")
	flag.StringVar(&json, "json", "", "JSON workload spec file, alternative way of providing name, repeat and activity information")
	var max int
	flag.IntVar(&max, "max-index", 0, "If given, 'INDEX' in devname is replaced with value of JOB_COMPLETION_INDEX % <max-index>")
	flag.Parse()

	if devnames == "" {
		wl.Devices = getDevices(devices)
	} else {
		wl.Devices = getDevnames(devnames, max)
	}
	wl.Profile = parseProfiles(activity)
	if len(wl.Profile) == 0 {
		log.Fatal("ERROR: no WL activity specified")
	}
	parseJSON(json, &wl)
	if wl.Name == "" {
		log.Fatal("ERROR: Workload name is missing")
	}
	return wl, socket
}

func main() {
	wl, socket := parseArgs()
	msg, err := json.MarshalIndent(wl, "", "\t")
	if err != nil {
		log.Fatalf("ERROR: internal WL marshaling error %v", err)
	}
	log.Printf("Workload: %v", string(msg))

	conn, err := net.Dial("unix", socket)
	if err != nil {
		log.Fatalf("ERROR: connection to 'fakedev-exporter' unix socket '%s' failed: %v", socket, err)
	}
	defer conn.Close()
	n, err := conn.Write(msg)
	if err != nil || n != len(msg) {
		log.Fatalf("ERROR: WL spec write (%d/%d bytes) to 'fakedev-exporter' failed: %v", n, len(msg), err)
	}
	// wait until server tells to exit with given exit code
	data := make([]byte, 8)
	n, err = conn.Read(data)
	if err != nil {
		log.Fatalf("ERROR: 'fakedev-exporter' socket read failed: %v", err)
	}
	retval := string(data[:n])
	ret, err := strconv.Atoi(retval)
	if err != nil {
		log.Fatalf("ERROR: could not parse 'fakedev-exporter' exit code '%s': %v", retval, err)
	}
	log.Printf("Exiting with code %d returned by server", ret)
	os.Exit(ret)
}
