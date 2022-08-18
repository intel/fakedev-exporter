Fakedev-exporter configurations
===============================

Here are example JSON specs for different fakedev-exporter configuration categories:
* [devices/](devices/)
  - Device type files (`-devtype` option)
  - PCI ID / device file name lists (`-devlist` option)
* [identity/](identity/)
  - Prometheus exporter identities to fake (`-identity` option)
* [workloads/](workloads/)
  - example workloads to simulate on the faked devices
    (`-wl-all`, `-wl-odd`, `-wl-even` server options, `-json` client option)

You need to specify at least device type, device list and exporter identity.
