// Copyright 2022 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"sort"
	"time"
)

const (
	wlExitOK    = "0"
	wlExitError = "1"
	wlMaxBatch  = 16 // how many WLs k8s could normally schedule between queries
)

// wlProfileT values are in percents and seconds
type wlProfileT struct {
	Load        int
	Fluctuation int
	Seconds     uint
}

type workloadInfoT struct {
	Name    string
	Repeat  uint
	Profile []wlProfileT
	Devices []string
	Limits  map[string]float64
}

// devProfileT values are ratios against device range, deadline,
// and seconds since activity start
type devProfileT struct {
	load        float64
	fluctuation float64
	deadline    time.Time
	seconds     time.Duration // time offset for looping
}

type workloadT struct {
	name     string
	conn     net.Conn
	activity int
	repeat   uint
	profile  []devProfileT
	devmap   map[int]bool
}

var (
	workload    []workloadT   = make([]workloadT, 0)
	connections chan net.Conn = make(chan net.Conn, wlMaxBatch)
)

func addWorkload(text []byte, devmap map[int]bool, conn net.Conn) bool {
	var (
		info workloadInfoT
		err  error
	)
	log.Printf("workload: %s\n", string(text))
	if err = json.Unmarshal(text, &info); err != nil {
		log.Printf("WARN, ignoring WL, unmarshaling its info JSON failed: %v", err)
		return false
	}
	if info.Name == "" {
		log.Printf("WARN, ignoring WL with invalid name '', ")
		return false
	}
	if len(info.Profile) == 0 {
		log.Printf("WARN, ignoring WL '%s' with no activity profile(s)", info.Name)
		return false
	}
	if len(info.Devices) > 0 {
		devmap = mapDevices(info.Devices)
	}
	if len(devmap) == 0 {
		log.Printf("WARN, ignoring WL '%s' with no mapped devices", info.Name)
		return false
	}
	if info.Limits != nil {
		log.Printf("TODO, ignoring WL '%s' limits until metric dependencies work", info.Name)
	}
	now := time.Now()
	total := time.Duration(0)
	profile := make([]devProfileT, len(info.Profile))
	for i, p := range info.Profile {
		// validate simulation values
		if p.Load < 0 || p.Load > 100 || p.Fluctuation < 0 || p.Fluctuation > 100 {
			log.Printf("WARN, ignoring WL where activity %d per-device load %d or %d fluctuation is not within 0-100",
				i, p.Load, p.Fluctuation)
			return false
		}
		if (p.Load-p.Fluctuation) < 0 || (p.Load+p.Fluctuation) > 100 {
			log.Printf("WARN, ignoring WL where activity %d per-device %d load +/- %d fluctuation is not within 0-100",
				i, p.Load, p.Fluctuation)
			return false
		}
		// time from given activity start
		var seconds time.Duration
		if p.Seconds > 0 {
			seconds, _ = time.ParseDuration(fmt.Sprintf("%ds", p.Seconds))
		} else {
			log.Printf("seconds <= 0, setting WL activity %d deadline to 1 day", i)
			seconds = 24 * time.Hour
		}
		// from first activity start
		total += seconds
		profile[i] = devProfileT{
			load:        float64(p.Load) / 100.0,
			fluctuation: float64(p.Fluctuation) / 100.0,
			deadline:    now.Add(total),
			seconds:     total,
		}
	}
	workload = append(workload, workloadT{
		name:    info.Name,
		conn:    conn,
		devmap:  devmap,
		profile: profile,
		repeat:  info.Repeat,
	})
	log.Printf("Loaded %gs workload '%s' to %d simulated devices", total.Seconds(), info.Name, len(devmap))
	return true
}

type filter func(int) bool

// loadWorkload() loads given workload intended to act as base load,
// for devices which index pass the filter
func loadWorkload(path string, devcount int, fn filter) {
	if path == "" {
		return
	}
	var (
		data []byte
		err  error
	)
	if data, err = os.ReadFile(path); err != nil {
		log.Fatalf("Unable to read WL info JSON file '%s': %v", path, err)
	}
	devmap := make(map[int]bool)
	for i := 0; i < devcount; i++ {
		if fn(i) {
			devmap[i] = true
		}
	}
	addWorkload(data, devmap, nil)
}

// listenForWorkloads() listens on given socket and pushes accepted
// WL connections to the related channel, as it's ran in its own go thread
func listenForWorkloads(path string) {
	os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		log.Fatalf("Unix socket '%s' listening failed: %v", path, err)
	}
	log.Printf("Listening on unix socket '%s'", path)
	for {
		conn, err := l.Accept()
		if err == nil {
			connections <- conn
			continue
		}
		log.Printf("Unix socket '%s' accept fail: %v", path, err)
	}
}

// acceptWorkloads() reads and adds all queued incoming workloads
func acceptWorkloads() {
	for {
		var c net.Conn
		select {
		case c = <-connections:
			break
		default:
			return
		}
		data := make([]byte, 1024)
		c.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
		count, err := c.Read(data)
		if count <= 0 || err != nil {
			log.Printf("Zero bytes or error from WL connection read: %d, %v", count, err)
		} else {
			log.Printf("New WL connected, with %d bytes spec\n", count)
			if addWorkload(data[:count], nil, c) {
				continue
			}
		}
		c.Write([]byte(wlExitError))
		c.Close()
	}
}

// addWorkloadsToMetric() adds load + fluctuation from each workload being
// simulated on given device, to the given metric value and returns the result
func addWorkloadsToMetric(dev int, value float64, limit limitT) float64 {
	scale := limit.Max - limit.Min
	for _, wl := range workload {
		if !wl.devmap[dev] {
			continue
		}
		load := wl.profile[wl.activity].load
		flux := wl.profile[wl.activity].fluctuation
		value += scale * (load + (rand.Float64()-0.5)*flux)
	}
	return value
}

// updateWorkloads() advances WL profile activity indexes when activities
// expire, and after last, either starts them from beginning, or removes
// WL when its activities count goes to zero.
//
// TODO: add WL per-metric updates after metric dependencies work
func updateWorkloads() {
	now := time.Now()
	rm := make([]int, 0)
	for i, wl := range workload {
		if wl.conn != nil {
			// TODO: this is really annoying, for disconnect detection
			// to work, it must try non-zero sized read with deadline
			// in *future* i.e. this adds delay to updates
			//
			// Unix module would allow checking connection state
			// (extract its FD, use syscalls to check the state),
			// but it's not in Golang standard library
			buf := make([]byte, 1)
			wl.conn.SetReadDeadline(time.Now().Add(time.Millisecond))
			_, err := wl.conn.Read(buf)
			if !errors.Is(err, os.ErrDeadlineExceeded) {
				log.Printf("WL-%d disconnected: %v\n", i, err)
				workload[i].conn = nil
				rm = append(rm, i)
				continue
			}
		}
		activity := wl.activity
		activities := len(wl.profile)
		// skip all expired activities
		for ; activity < activities; activity++ {
			if now.Before(wl.profile[activity].deadline) {
				break
			}
		}
		if activity == wl.activity {
			continue
		}
		log.Printf("WL-%d ('%s') activity %d/%d expired", i, wl.name, activity, activities)
		if activity < activities {
			workload[i].activity = activity
			continue
		}
		if wl.repeat == 1 {
			// all done, mark WL for removal
			rm = append(rm, i)
			continue
		}
		// decrease repeat counter, when not looping forever
		if wl.repeat > 0 {
			workload[i].repeat--
		}
		// reset profile deadlines and its activity index
		for activity = 0; activity < activities; activity++ {
			seconds := wl.profile[activity].seconds
			workload[i].profile[activity].deadline = now.Add(seconds)
		}
		workload[i].activity = 0
		log.Printf("WL-%d ('%s') activity looped", i, wl.name)
	}
	removeWorkloads(rm)
}

// removeWorkloads() removes WLs with given indexes from the WL list.
// Note that WL order is not preserved, as it should not matter
func removeWorkloads(rm []int) {
	if len(rm) == 0 {
		return
	}
	// reverse sort, so last ones are replaced first
	sort.Slice(rm, func(i, j int) bool { return rm[i] > rm[j] })
	// https://github.com/golang/go/wiki/SliceTricks#delete
	count := len(workload)
	for i, wli := range rm {
		offset := count - i - 1
		log.Printf("Removing WL-%d ('%s')", wli, workload[wli].name)
		if workload[wli].conn != nil {
			c := workload[wli].conn
			c.Write([]byte(wlExitOK))
			c.Close()
		}
		workload[wli] = workload[offset]
		// make sure moved WL gets GCed
		workload[offset].devmap = nil
	}
	workload = workload[:count-len(rm)]
}
