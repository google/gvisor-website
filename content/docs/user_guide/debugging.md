+++
title = "Debugging"
weight = 120
+++

To enable debug and system call logging, add the `runtimeArgs` below to your
[Docker](../docker/) configuration (`/etc/docker/daemon.json`):

```json
{
    "runtimes": {
        "runsc": {
            "path": "/usr/local/bin/runsc",
            "runtimeArgs": [
                "--debug-log=/tmp/runsc/",
                "--debug",
                "--strace"
            ]
       }
    }
}
```


> Note: the last `/` in `--debug-log` is needed to interpret it as a directory.
> Then each `runsc` command executed will create a separate log file.
> Otherwise, log messages from all commands will be appended to the same file.

You may also want to pass `--log-packets` to troubleshoot network problems. Then
restart the Docker daemon:

```bash
sudo systemctl restart docker
```

Run your container again, and inspect the files under `/tmp/runsc`. The log file
with name `*.boot` will contain the strace logs from your application, which can
be useful for identifying missing or broken system calls in gVisor. If you're
having problems to start the container, the file with name `*.create` may have
the reason for the failure.
