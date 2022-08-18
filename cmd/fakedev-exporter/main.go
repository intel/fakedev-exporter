// Copyright 2022 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

const (
	project   = "fakedev-exporter"
	version   = "v0.1"
	metricURL = "/metrics"
)

var (
	// device labels, metric limits and what metrics to output
	devinfo devinfoT
	// [device][metric]: value
	device []map[string]float64
	mutex  sync.Mutex
)

// mapDevices() maps device file name to device array index
func mapDevices(names []string) map[int]bool {
	errors := 0
	devmap := make(map[int]bool, len(devinfo.devicemap))
	for _, name := range names {
		if dev, exists := devinfo.devicemap[name]; exists {
			devmap[dev] = true
		} else {
			errors++
		}
	}
	if errors > 0 {
		log.Printf("WARN, %d of WL device file names (%v) did NOT match server ones: %v", errors, names, devinfo.devicemap)
		return nil
	}
	return devmap
}

// runSimulation updates all metrics in devices. First it sets minimum value to
// a metric and then asks each workload to add their own values on top of that,
// with end result then being limited to maximum value.
func runSimulation() {
	for dev := 0; dev < len(device); dev++ {
		limited := make([]string, 0)
		// TODO: use ordered metrics list instead of (random order) map
		for metric, limit := range devinfo.metricLimits {
			value := addWorkloadsToMetric(dev, limit.Min, limit)
			if value < limit.Min {
				// limits differ between metrics which should help to identify them
				limited = append(limited, fmt.Sprintf("%g < %g", value, limit.Min))
				value = limit.Min
			}
			if value > limit.Max {
				limited = append(limited, fmt.Sprintf("%g > %g", value, limit.Max))
				value = limit.Max
			}
			device[dev][metric] = value
		}
		if len(limited) > 0 {
			log.Printf("Device-%d metrics needed limiting: %v", dev, strings.Join(limited, ", "))
		}
	}
}

func writeMetric(w http.ResponseWriter, dev int, metric string, mvalue float64) {
	comma := false
	labelSets := [][]labelPairT{
		devinfo.deviceLabels[dev], devinfo.metricLabels[metric],
	}
	fmt.Fprintf(w, "%s{", metric)
	for _, labels := range labelSets {
		for _, label := range labels {
			if comma {
				fmt.Fprintf(w, ", %s=\"%s\"", label.name, label.value)
			} else {
				fmt.Fprintf(w, "%s=\"%s\"", label.name, label.value)
				comma = true
			}
		}
	}
	fmt.Fprintf(w, "} %g\n", mvalue)
}

func requestCheck(r *http.Request) int {
	if r.Method != http.MethodGet {
		return http.StatusMethodNotAllowed
	}
	if r.URL.Path != metricURL {
		return http.StatusNotFound
	}
	if r.Body != http.NoBody {
		return http.StatusBadRequest
	}
	return http.StatusOK
}

func exporter(w http.ResponseWriter, r *http.Request) {
	mutex.Lock()
	defer mutex.Unlock()
	if status := requestCheck(r); status != http.StatusOK {
		w.WriteHeader(status)
		return
	}
	// run simulation related items
	acceptWorkloads()
	runSimulation()
	updateWorkloads()

	// report results
	fmt.Fprintf(w, "# %s %s\n", project, version)
	for dev := 0; dev < len(device); dev++ {
		for _, metric := range devinfo.output {
			if value, exists := device[dev][metric]; exists {
				writeMetric(w, dev, metric, value)
			}
		}
	}
}

func listenPrometheus(address string) {
	http.HandleFunc(metricURL, exporter)
	log.Printf("Listening on %s%s", address, metricURL)
	log.Fatal(http.ListenAndServe(address, nil))
}

func main() {
	log.Printf("%s %s", project, version)
	var devtype, devlist, idfile, address, wlEven, wlOdd, wlAll, socket string
	var count int
	flag.StringVar(&address, "address", ":9999", "Address to listen for metric queries")
	flag.IntVar(&count, "count", 1, "Number of devices (of specified type) to simulate")
	flag.StringVar(&devtype, "devtype", "devtype.json", "Name of JSON config file for device type labels + metric limits")
	flag.StringVar(&devlist, "devlist", "devlist.json", "Name of JSON config file for per-device instance labels")
	flag.StringVar(&idfile, "identity", "identity.json", "Name of JSON config file for metric exporter identity")
	flag.StringVar(&socket, "socket", "/tmp/"+project, "Unix socket for workload communication")
	flag.StringVar(&wlEven, "wl-even", "", "Name of JSON file specifying workload to run on even numbered devices")
	flag.StringVar(&wlAll, "wl-all", "", "Name of JSON file specifying workload to run on all devices")
	flag.StringVar(&wlOdd, "wl-odd", "", "Name of JSON file specifying workload to run on odd numbered devices")
	flag.Parse()

	devinfo = getDevinfo(count, devtype, devlist, idfile)
	devcount := len(devinfo.deviceLabels)

	// allocate current metric values and show device labels
	device = make([]map[string]float64, devcount)
	log.Print("Initial devinfo labels:")
	for dev := 0; dev < devcount; dev++ {
		log.Printf("+ [%d]", dev)
		for _, label := range devinfo.deviceLabels[dev] {
			log.Printf("  - %s='%s'\n", label.name, label.value)
		}
		device[dev] = make(map[string]float64)
	}
	loadWorkload(wlEven, devcount, func(i int) bool { return i%2 == 0 })
	loadWorkload(wlOdd, devcount, func(i int) bool { return i%2 != 0 })
	loadWorkload(wlAll, devcount, func(i int) bool { return true })

	const umask = 07077
	old := syscall.Umask(umask)
	log.Printf("Umask: %04o -> %04o", old, umask)

	go listenForWorkloads(socket)
	go listenPrometheus(address)

	// exit with 0 when asked nicely to terminate
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	log.Printf("Got signal %d => terminating", <-sig)
	os.Exit(0)
}
