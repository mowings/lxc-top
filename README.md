# lxc-top: Show which lxc containers are hogging resources

`lxc-top` is a simple top-like program to show top lxc-containers by cpu or memory usage. This can
be useful when tracking down excessive container resource usage.

To install, simply download a release, untar it, copy the lxc-top binary to somehwere on your path, and run 

    sudo lxc-top

Toggle between cpu/memory usage sorts by pressing `s`, and quit by pressing `q`.

## Building

You will need a relatively recent version of go. lxc-top is build with [gb](https://getgb.io/), but you can use standard go tools
by executing `env.sh` in the project directory to change your GOPATH:

    . env.sh

After the GOPATH change, ordinary go builds should work fine.



