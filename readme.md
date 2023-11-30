# fix android net
 * fix x/mobile: Calling net.InterfaceAddrs() fails on Android SDK 30
 * network: route ip+net: netlinkrib: permission denied

# releas
 go version 1.21.4
 [here](https://github.com/nfsq246/fix-android-net/releases/tag/v0.0.1)
# build
 1. download
    ```
    wget https://go.dev/dl/go1.21.4.src.tar.gz
    tar -xzf go1.21.4.linux-amd64.tar.gz
    git clone https://github.com/nfsq246/fix-android-net.git
    ```
 2. copy
    ```
    cp fix-android-net/interface_android.go go/src/net/
    cp fix-android-net/netlink_android.go go/src/syscall/
    ```
 3. edit
    ```
    vim go/src/net/interface_linux.go
    ```

    add "//go:build !android" on line 4

    ``` go
    // license that can be found in the LICENSE file.
    +++ //go:build !android
    package net

    import (
        "fmt"
        "os/exec"
        "testing"
    )
    ```
    ```
    vim go/src/net/interface_linux_test.go
    ```
    add "//go:build !android" on line 4
    ``` go
    // license that can be found in the LICENSE file.
    +++ //go:build !android
    package net

    import (
        "os"
        "syscall"
        "unsafe"
    )
    ```
    ```
    vim go/src/syscall/netlink_linux.go
    ```
    add "//go:build !android" on line 6

    ``` go
    // Netlink sockets and messages
    +++ //go:build !android
    package syscall

    import (
        "sync"
        "unsafe"
    )
    ```
 4. build source code
    ```
    ./go/src/all.bash
    ## you will see success build
    tar -zcvf go1.21.4.net.linux-amd64.tar.gz go
    ```
    
# references
[github.com/wlynxg/anet](https://github.com/wlynxg/anet)

[github.com/golang/go/issues/40569](https://github.com/golang/go/issues/40569)
[1](https://github.com/golang/go/issues/40569#issuecomment-1600898227)
[2](https://github.com/golang/go/issues/40569#issuecomment-1603916131)
[3](https://github.com/golang/go/issues/40569#issuecomment-1614781739)