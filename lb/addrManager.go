package main

import "net"

import "fmt"

func newAddrManager(addrRange string) (*AddrManager, error) {
	_, ipnet, err := net.ParseCIDR(addrRange)
	if err != nil {
		return nil, err
	}
	var ip [16]byte
	copy(ip[:], ipnet.IP)
	return &AddrManager{
		AddrRange: ipnet,
		Current:   ip,
	}, nil
}

type AddrManager struct {
	AddrRange *net.IPNet
	Current   [16]byte
}

func (am *AddrManager) releaseIP() (net.IP, error) {
	am.Current[15]++
	addr := net.IP(am.Current[:])
	if !am.AddrRange.Contains(addr) {
		return nil, fmt.Errorf("Insufficient address")
	}
	return addr, nil
}
