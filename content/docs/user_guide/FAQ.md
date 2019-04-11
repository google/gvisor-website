+++
title = "FAQ"
weight = 1000
+++

### My container runs fine with `runc` but fails with `runsc`

If you’re having problems running a container with `runsc` it’s most likely due
to a compatibility issue or a missing feature in gVisor. See
[Debugging](../debugging/).

### When I run my container, docker fails with: `flag provided but not defined: -console`

You're using an old version of Docker. See [Docker Quick Start](../docker/).

### I can’t see a file copied with: `docker cp`

For performance reasons, gVisor caches directory contents, and therefore it may
not realize a new file was copied to a given directory. To invalidate the cache
and force a refresh, create a file under the directory in question and list the
contents again.

As a workaround, shared root filesystem can be enabled. See [Filesystem](../filesystem/).

This bug is tracked in [bug #4](https://github.com/google/gvisor/issues/4).

Note that `kubectl cp` works because it does the copy by exec'ing inside the
sandbox, and thus gVisor's internal cache is made aware of the new files and
directories.

### What's the security model?

See the [Security Model](../../architecture_guide/security/).
