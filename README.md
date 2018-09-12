# lxc-top: Show which lxc containers are hogging resources

`lxc-top` is a simple top-like program to show top lxc-containers by cpu or memory usage. This can
be useful when tracking down excessive container resource usage.

To install, simply download a release, untar it, copy the lxc-top binary to somehwere on your path, and run it:

    wget https://github.com/mowings/lxc-top/releases/download/1.0.1a/lxc-top-1.0.1.tgz
    tar -xzvf lxc-top-1.0.1.tgz
    sudo cp lxc-top /usr/local/bin
    sudo lxc-top

Toggle between cpu/memory usage sorts by pressing `s`, and quit by pressing `q`.

Unlike the `lxc-top` program that comes bundled with lxc, this version show cpu percentages as snapshotted between
runs, as opposed to simple cumulative cpu seconds per container. This will give you a better idea of current cpu usage in real-time.

Note that cpu percentages will frequently go above 100% for containers hosted on multi-core systems. This is normal. 

## Building

You will need a relatively recent version of go. lxc-top is build with [gb](https://getgb.io/), but you can use standard go tools
by executing `env.sh` in the project directory to change your GOPATH:

    . env.sh

After the GOPATH change, ordinary go builds should work fine.



