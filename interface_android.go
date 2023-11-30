// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import (
	"os"
	"syscall"
	"unsafe"
	"unicode/utf8"
)
type ifReq [40]byte
// If the ifindex is zero, interfaceTable returns mappings of all
// network interfaces. Otherwise it returns a mapping of a specific
// interface.
func interfaceTable(ifindex int) ([]Interface, error) {
	tab, err := syscall.NetlinkRIB(syscall.RTM_GETADDR, syscall.AF_UNSPEC)
	if err != nil {
		return nil, os.NewSyscallError("netlinkrib", err)
	}
	msgs, err := syscall.ParseNetlinkMessage(tab)
	if err != nil {
		return nil, os.NewSyscallError("parsenetlinkmessage", err)
	}

	var ift []Interface
	im := make(map[uint32]struct{})
loop:
	for _, m := range msgs {
		switch m.Header.Type {
		case syscall.NLMSG_DONE:
			break loop
		case syscall.RTM_NEWADDR:
			ifam := (*syscall.IfAddrmsg)(unsafe.Pointer(&m.Data[0]))
			if _, ok := im[ifam.Index]; ok {
				continue
			} else {
				im[ifam.Index] = struct{}{}
			}

			if ifindex == 0 || ifindex == int(ifam.Index) {
				ifi := newLink(ifam)
				if ifi != nil {
					ift = append(ift, *ifi)
				}
				if ifindex == int(ifam.Index) {
					break loop
				}
			}
		}
	}

	return ift, nil
}

const (
	// See linux/if_arp.h.
	// Note that Linux doesn't support IPv4 over IPv6 tunneling.
	sysARPHardwareIPv4IPv4 = 768 // IPv4 over IPv4 tunneling
	sysARPHardwareIPv6IPv6 = 769 // IPv6 over IPv6 tunneling
	sysARPHardwareIPv6IPv4 = 776 // IPv6 over IPv4 tunneling
	sysARPHardwareGREIPv4  = 778 // any over GRE over IPv4 tunneling
	sysARPHardwareGREIPv6  = 823 // any over GRE over IPv6 tunneling
)

func newLink(ifam *syscall.IfAddrmsg) *Interface {
	ift := &Interface{Index: int(ifam.Index)}

	name, err := indexToName(ifam.Index)
	if err != nil {
		return nil
	}
	ift.Name = name

	mtu, err := nameToMTU(name)
	if err != nil {
		return nil
	}
	ift.MTU = mtu

	flags, err := nameToFlags(name)
	if err != nil {
		return nil
	}
	ift.Flags = flags
	return ift
}

func linkFlags(rawFlags uint32) Flags {
	var f Flags
	if rawFlags&syscall.IFF_UP != 0 {
		f |= FlagUp
	}
	if rawFlags&syscall.IFF_RUNNING != 0 {
		f |= FlagRunning
	}
	if rawFlags&syscall.IFF_BROADCAST != 0 {
		f |= FlagBroadcast
	}
	if rawFlags&syscall.IFF_LOOPBACK != 0 {
		f |= FlagLoopback
	}
	if rawFlags&syscall.IFF_POINTOPOINT != 0 {
		f |= FlagPointToPoint
	}
	if rawFlags&syscall.IFF_MULTICAST != 0 {
		f |= FlagMulticast
	}
	return f
}

// If the ifi is nil, interfaceAddrTable returns addresses for all
// network interfaces. Otherwise it returns addresses for a specific
// interface.
func interfaceAddrTable(ifi *Interface) ([]Addr, error) {
	tab, err := syscall.NetlinkRIB(syscall.RTM_GETADDR, syscall.AF_UNSPEC)
	if err != nil {
		return nil, os.NewSyscallError("netlinkrib", err)
	}
	msgs, err := syscall.ParseNetlinkMessage(tab)
	if err != nil {
		return nil, os.NewSyscallError("parsenetlinkmessage", err)
	}

	var ift []Interface
	if ifi == nil {
		var err error
		ift, err = interfaceTable(0)
		if err != nil {
			return nil, err
		}
	}
	ifat, err := addrTable(ift, ifi, msgs)
	if err != nil {
		return nil, err
	}
	return ifat, nil
}

func addrTable(ift []Interface, ifi *Interface, msgs []syscall.NetlinkMessage) ([]Addr, error) {
	var ifat []Addr
loop:
	for _, m := range msgs {
		switch m.Header.Type {
		case syscall.NLMSG_DONE:
			break loop
		case syscall.RTM_NEWADDR:
			ifam := (*syscall.IfAddrmsg)(unsafe.Pointer(&m.Data[0]))
			if len(ift) != 0 || ifi.Index == int(ifam.Index) {
				attrs, err := syscall.ParseNetlinkRouteAttr(&m)
				if err != nil {
					return nil, os.NewSyscallError("parsenetlinkrouteattr", err)
				}
				ifa := newAddr(ifam, attrs)
				if ifa != nil {
					ifat = append(ifat, ifa)
				}
			}
		}
	}
	return ifat, nil
}

func newAddr(ifam *syscall.IfAddrmsg, attrs []syscall.NetlinkRouteAttr) Addr {
	var ipPointToPoint bool
	// Seems like we need to make sure whether the IP interface
	// stack consists of IP point-to-point numbered or unnumbered
	// addressing.
	for _, a := range attrs {
		if a.Attr.Type == syscall.IFA_LOCAL {
			ipPointToPoint = true
			break
		}
	}
	for _, a := range attrs {
		if ipPointToPoint && a.Attr.Type == syscall.IFA_ADDRESS {
			continue
		}
		switch ifam.Family {
		case syscall.AF_INET:
			return &IPNet{IP: IPv4(a.Value[0], a.Value[1], a.Value[2], a.Value[3]), Mask: CIDRMask(int(ifam.Prefixlen), 8*IPv4len)}
		case syscall.AF_INET6:
			ifa := &IPNet{IP: make(IP, IPv6len), Mask: CIDRMask(int(ifam.Prefixlen), 8*IPv6len)}
			copy(ifa.IP, a.Value[:])
			return ifa
		}
	}
	return nil
}

// interfaceMulticastAddrTable returns addresses for a specific
// interface.
func interfaceMulticastAddrTable(ifi *Interface) ([]Addr, error) {
	ifmat4 := parseProcNetIGMP("/proc/net/igmp", ifi)
	ifmat6 := parseProcNetIGMP6("/proc/net/igmp6", ifi)
	return append(ifmat4, ifmat6...), nil
}

func parseProcNetIGMP(path string, ifi *Interface) []Addr {
	fd, err := open(path)
	if err != nil {
		return nil
	}
	defer fd.close()
	var (
		ifmat []Addr
		name  string
	)
	fd.readLine() // skip first line
	b := make([]byte, IPv4len)
	for l, ok := fd.readLine(); ok; l, ok = fd.readLine() {
		f := splitAtBytes(l, " :\r\t\n")
		if len(f) < 4 {
			continue
		}
		switch {
		case l[0] != ' ' && l[0] != '\t': // new interface line
			name = f[1]
		case len(f[0]) == 8:
			if ifi == nil || name == ifi.Name {
				// The Linux kernel puts the IP
				// address in /proc/net/igmp in native
				// endianness.
				for i := 0; i+1 < len(f[0]); i += 2 {
					b[i/2], _ = xtoi2(f[0][i:i+2], 0)
				}
				i := *(*uint32)(unsafe.Pointer(&b[:4][0]))
				ifma := &IPAddr{IP: IPv4(byte(i>>24), byte(i>>16), byte(i>>8), byte(i))}
				ifmat = append(ifmat, ifma)
			}
		}
	}
	return ifmat
}

func parseProcNetIGMP6(path string, ifi *Interface) []Addr {
	fd, err := open(path)
	if err != nil {
		return nil
	}
	defer fd.close()
	var ifmat []Addr
	b := make([]byte, IPv6len)
	for l, ok := fd.readLine(); ok; l, ok = fd.readLine() {
		f := splitAtBytes(l, " \r\t\n")
		if len(f) < 6 {
			continue
		}
		if ifi == nil || f[1] == ifi.Name {
			for i := 0; i+1 < len(f[2]); i += 2 {
				b[i/2], _ = xtoi2(f[2][i:i+2], 0)
			}
			ifma := &IPAddr{IP: IP{b[0], b[1], b[2], b[3], b[4], b[5], b[6], b[7], b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15]}}
			ifmat = append(ifmat, ifma)
		}
	}
	return ifmat
}



func ioctl(fd int, req uint, arg unsafe.Pointer) error {
	_, _, e1 := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(arg))
	if e1 != 0 {
		return e1
	}
	return nil
}

func indexToName(index uint32) (string, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return "", err
	}
	defer syscall.Close(fd)

	var ifr ifReq
	*(*uint32)(unsafe.Pointer(&ifr[syscall.IFNAMSIZ])) = index
	err = ioctl(fd, syscall.SIOCGIFNAME, unsafe.Pointer(&ifr[0]))
	if err != nil {
		return "", err
	}

	return string(trim(ifr[:syscall.IFNAMSIZ], "\x00")), nil
}
func trim(s []byte, cutset string) []byte {
	if len(s) == 0 {
		// This is what we've historically done.
		return nil
	}
	if cutset == "" {
		return s
	}
	if len(cutset) == 1 && cutset[0] < utf8.RuneSelf {
		return trimLeftByte(trimRightByte(s, cutset[0]), cutset[0])
	}
	if as, ok := makeASCIISet(cutset); ok {
		return trimLeftASCII(trimRightASCII(s, &as), &as)
	}
	return trimLeftUnicode(trimRightUnicode(s, cutset), cutset)
}
type asciiSet [8]uint32
func (as *asciiSet) contains(c byte) bool {
	return (as[c/32] & (1 << (c % 32))) != 0
}
func trimLeftByte(s []byte, c byte) []byte {
	for len(s) > 0 && s[0] == c {
		s = s[1:]
	}
	if len(s) == 0 {
		// This is what we've historically done.
		return nil
	}
	return s
}
func trimRightByte(s []byte, c byte) []byte {
	for len(s) > 0 && s[len(s)-1] == c {
		s = s[:len(s)-1]
	}
	return s
}
func makeASCIISet(chars string) (as asciiSet, ok bool) {
	for i := 0; i < len(chars); i++ {
		c := chars[i]
		if c >= utf8.RuneSelf {
			return as, false
		}
		as[c/32] |= 1 << (c % 32)
	}
	return as, true
}
func trimLeftASCII(s []byte, as *asciiSet) []byte {
	for len(s) > 0 {
		if !as.contains(s[0]) {
			break
		}
		s = s[1:]
	}
	if len(s) == 0 {
		// This is what we've historically done.
		return nil
	}
	return s
}
func trimRightASCII(s []byte, as *asciiSet) []byte {
	for len(s) > 0 {
		if !as.contains(s[len(s)-1]) {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}
func trimLeftUnicode(s []byte, cutset string) []byte {
	for len(s) > 0 {
		r, n := rune(s[0]), 1
		if r >= utf8.RuneSelf {
			r, n = utf8.DecodeRune(s)
		}
		if !containsRune(cutset, r) {
			break
		}
		s = s[n:]
	}
	if len(s) == 0 {
		// This is what we've historically done.
		return nil
	}
	return s
}
func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}
func trimRightUnicode(s []byte, cutset string) []byte {
	for len(s) > 0 {
		r, n := rune(s[len(s)-1]), 1
		if r >= utf8.RuneSelf {
			r, n = utf8.DecodeLastRune(s)
		}
		if !containsRune(cutset, r) {
			break
		}
		s = s[:len(s)-n]
	}
	return s
}
func nameToMTU(name string) (int, error) {
	// Leave room for terminating NULL byte.
	if len(name) >= syscall.IFNAMSIZ {
		return -1, syscall.EINVAL
	}

	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	defer syscall.Close(fd)

	var ifr ifReq
	copy(ifr[:], name)
	err = ioctl(fd, syscall.SIOCGIFMTU, unsafe.Pointer(&ifr[0]))
	if err != nil {
		return -1, err
	}

	return int(*(*int32)(unsafe.Pointer(&ifr[syscall.IFNAMSIZ]))), nil
}

func nameToFlags(name string) (Flags, error) {
	// Leave room for terminating NULL byte.
	if len(name) >= syscall.IFNAMSIZ {
		return 0, syscall.EINVAL
	}

	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return 0, err
	}
	defer syscall.Close(fd)

	var ifr ifReq
	copy(ifr[:], name)
	err = ioctl(fd, syscall.SIOCGIFFLAGS, unsafe.Pointer(&ifr[0]))
	if err != nil {
		return 0, err
	}

	return linkFlags(*(*uint32)(unsafe.Pointer(&ifr[syscall.IFNAMSIZ]))), nil
}