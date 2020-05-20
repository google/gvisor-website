+++
title = "Testing Guide"
weight = 10
+++
gVisor has an extensive test suite that covers unit tests for internal data
structions, as well as syscall tests for testing Linux compatibility.

# Running tests

Tests are built and run using [Bazel](https://bazel.build/) and are specified
in `BUILD` files in various directories thoughout the source tree. Bazel uses
the directory structure and `BUILD` files to define a "package" (See the
[Bazel documentation](https://docs.bazel.build/versions/master/build-ref.html#packages_targets)).
The entire repository is referred to as a Bazel "workspace".

The entire test suite can be run with the following command. This command is
using the double slash (//) operator to specify the root of the workspace and
the elipsis (...) operator as a wildcard to specify that all tests under the
root workspace should be run. This will include both unit tests and syscall
tests.

```
bazel test //...
```

Running specific test suites or targets is done using variations of the
`bazel test` command.

# Running unit tests

Running unit tests is fairly straightforward in the Bazel style. Running all
unit tests is a matter of specifying all of the Go base packages.

```
bazel test //runsc/... //pkg/... //tools/...
```

You can also specify individual targets to run tests.

```
bazel test //runsc/boot:boot_test
```

# Running syscall tests

Syscall tests are used to verify the compatibility between gVisor and Linux
behavior. They are found under the `test/syscalls` directory in the project
root.  Limiting test execution to the syscall tests can be done by specifying
them as a target.

```
bazel test //test/syscalls/...
```

Individual syscall test targets can be found in the Bazel `BUILD` files under
`test/syscalls`. For instance `test/syscalls/linux/BUILD`.

The targets specified in the `BUILD` file will not include any actual tests. For example,
running `bazel test test/syscalls/linux:exec_test` will result in the
following:

```
$ bazel test //test/syscalls:exec_test
...
ERROR: No test targets were found, yet testing was requested
...
```

The actual test targets are generated for Linux and for each gVisor platform
(see [Platforms](/docs/user_guide/platforms/)) and are of the form `<BUILD
target>_native` for Linux and `<BUILD target>_runsc_<platform>` for gVisor
tests, and fall under the `test/syscalls` package. For example, to run the
`exec_test` on Linux you would run the following:

```
$ bazel test //test/syscalls:exec_test_native
```

To run the gVisor version of the test on the ptrace platform, you would run the
following:

```
$ bazel test //test/syscalls:exec_test_runsc_ptrace
```
