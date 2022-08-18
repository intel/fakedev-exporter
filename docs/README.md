
Fakedev-exporter design
=======================

NOTE: Information below describes the eventual design goals, not
necessarily the current design.

Contents:
* [Terminology](#terminology)
* [Command line options](#command-line-options)
* [Metric exporting](#metric-exporting)
* [Workloads](#workloads)
* [Workload simulation](#workload-simulation)
* [Device simulation](#device-simulation)
* [Initialization](#initialization)
* [Threading](#threading)


Terminology
-----------

* WL = workload


Command line options
--------------------

Options allow setting following:

* Configuration file for device simulation (devices + metrics info)
* Configuration file for exporter identity (metric and label mapping)
* Configuration file(s) for device base workload
* Metric exporting port number


Metric exporting
----------------

HTTP-server thread(s) answer metric queries.

When queried, it gets metrics values and their labels for each device
+ WL, and outputs that back to the HTTP connection in Prometheus ASCII
format.  WL metrics are used for per-pod/-container metrics.


Workloads
---------

Fake WL program takes following options:
* Required:
  * WL (pod) name
  * WL activity profile
  * Glob pattern for matching device names from file system
  * Regexp for extracting device indices from pattern matches
* Optional:
  * Device names (in case device plugin is not used)
    - `sh -c fakedev-workload --devnames cardINDEX --max-index 120 ...`
    - Where INDEX is replaced with JOB_COMPLETION_INDEX % max-index, see:
      https://kubernetes.io/docs/tasks/job/indexed-parallel-processing-static/
  * Metric limit values

And does following:
* Find out which device(s) are mapped to its container,
  if device names are not explicitly specified
* Connect to "fakedev-exporter" server
* Provide device indices and option values to server
* Sleep waiting a reply telling WL to terminate
  - Or if connection drops, exit with an error
* Log the reply message, and exit with given value


Workload simulation
-------------------

* Loads base workloads at startup
* Accepts connections from workload (WL) containers at run-time,
  and notices when they go away (connection drops)
* Adds provided activity profile for the WL, or logs error + tells WL
  to exit(1) if profile values were invalid
* Maps metric limit names based on identity information
* Maintains list of currently active WLs (each containing their
  per-device metric state), an activity profile, list of devices
  and which workloads are mapped to them
* WL can also specify what are its memory and runtime limitations
* If either GPU or WL metric limit is reached, tells WL to exit(1)
  with "Limit <X> reached, terminated" log message
* When end of activity list is reached, tells WL to exit(0) with OK msg
* Drops WL and its connection after telling it to exit

Activity profile info:
* Value function; either random, or random increase
* Value range and time period, where range can be either absolute
  (e.g. memory usage), or as share of max value (e.g. device usage)

Relevant profile metrics are:
* Device usage, 0-1
* Memory usage, in bytes
* Optionally also:
  * RAS / device hang, as counters
  * Memory BW, as bytes / second
  * Device interconnect BW usage

If WL specifies device max frequency + capability, time and metrics
values are scaled by simulated device values when added.


Device simulation
-----------------

Program main loop runs the simulation loop at given interval:
* Go through each metric in each active WL and sum their current
  metric values together, for each device associated with them
  - If WL addition crosses device metric termination limit,
    tell WL to terminate and remove WL
* Constrain resulting per-device metric values to specified min-max range
  and update current per-device metric values accordingly
* Update dependent metrics values based on configured metric relations
* Update WL metric values and if they cross WL metric termination limit,
  tell WL to terminate + remove it
* Check activity deadlines for each WL, and advance them or drop the WL


Initialization
--------------

At start, this reads device configuration file specifying:
* Device and sub-device count + potential other topology info
* Min and max values for each metric, and:
  - which metrics are for device, and which for sub-device
  - which metric limits cause WL termination
* Relations for dependent metrics
* Device label information added to all metrics
* Multi-value per-metric labels, i.e. extra metric dimensions
* Device capability information for scaling WL values
* Device file name -> index mapping

And an exporter identity file specifying:
* Device label name mapping (which ones to output)
* Single-value label info to add to specific metrics
* Metric name mapping (which ones to output)

Device + WL metric names, and their labels are then mapped based on
this information.  That allows simulating output from a given exporter
to be separated from actual metric simulation.

Then a list of devices / sub-devices is created, each with list of
specified primary & dependent metrics, set to their minimum and
dependent values.

Potential dependency rules for GPU metrics could be following:
* GPU hang -> 100% freq time
* Engine usage -> GPU frequency change ratio
* Mem / fabric BW usage -> mem frequency change ratio
* Frequency / engine / BW usage  -> GPU / mem power change ratio
* Power -> temperature change ratio + delay
* Temperature limit -> throttling time
* Throttling time -> Frequency / BW limit


Threading
---------

Currently there are separate Go routines for:
* structure initializations, creating the other threads, and
  termination signal handling
* handling incoming workload connections
* handling HTTP metric requests

First one does its work before other routines start and then waits
until signaled to exit. Incoming connections are Listen()ed in a loop
and queued to a channel. Therefore neither handles shared data that
would need locking.

Eventually there will be also a separate Go routine for metrics
simulation, but currently everything related to metrics is done from
the HTTP metric requests handler.  That calls functions to:
* Check for new workloads in the incoming workload connections channel,
* Simulate device(s) load based on workload specs + update device metrics,
* Update status for the workloads, and
* Output metrics

Golang HTTP server module uses go routines to parallelize handling of
parallel requests, so request handler uses mutex to protect /
serialize access to workload and metric related data.
