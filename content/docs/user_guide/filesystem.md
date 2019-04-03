+++
title = "Filesystem"
weight = 50
+++
gVisor accesses the filesystem through a file proxy, called gofer. The gofer runs
as a separate process, that is isolated from the sandbox. They communicate using
the 9P protocol. For a more detailed explanation see
[Overview > Gofer](../architecture_guide/overview/#gofer)

## Sandbox Overlay

To completely isolate the host filesystem from the sandbox, you can set a writable
tmpfs overlay on top of the entire filesystem. All modifications are saved to the
overlay, keeping the host filesystem unmodified.

> Note that all created and modified files are stored in memory inside the sandbox.

Add the following `runtimeArgs` to your Docker configuration
(`/etc/docker/daemon.json`) and restart the Docker daemon:

```json
{
    "runtimes": {
        "runsc": {
            "path": "/usr/local/bin/runsc",
            "runtimeArgs": [
                "--overlay"
            ]
       }
    }
}
```

## Shared RootFS

The root FS is where the image is extract and not expected to be modified externally.
This allows for some optimizations to take place, like skipping checks to determine
if a directory has changed since the last time it was cached. If you need to `docker cp`
files inside the root FS, you may want to enable shared mode. Just be aware that file
system access will be slower due to the extra checks that are required.

> Note: External mounts are always shared.

Add the following `runtimeArgs` to your Docker configuration
(`/etc/docker/daemon.json`) and restart the Docker daemon:

```json
{
    "runtimes": {
        "runsc": {
            "path": "/usr/local/bin/runsc",
            "runtimeArgs": [
                "--file-access=shared"
            ]
       }
    }
}
```
