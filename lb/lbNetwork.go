package main

import (
	"log"
	"net"
)

func newLBNetwork(config LBNetworkConfig) (*LBNetwork, error) {
	address, err := net.ResolveUDPAddr("udp", config.Network)
	if err != nil {
		return nil, err
	}
	source, err := net.ResolveUDPAddr("udp", config.Source)
	if err != nil {
		return nil, err
	}
	sender, err := net.DialUDP("udp", source, address)
	if err != nil {
		return nil, err
	}
	receiver, err := net.ListenMulticastUDP("udp", nil, address)
	if err != nil {
		return nil, err
	}
	ln := &LBNetwork{
		config:   config,
		source:   source.IP,
		sender:   sender,
		receiver: receiver,
		handler:  []func([]byte, net.IP){},
	}
	go func() {
		buffer := make([]byte, 9000)
		for {
			n, remote, err := ln.receiver.ReadFromUDP(buffer)
			if err != nil {
				log.Printf("error: %s\n", err)
			}
			if !remote.IP.Equal(ln.source) {
				buf := make([]byte, n)
				copy(buf, buffer[:n])
				for _, handler := range ln.handler {
					go handler(buf, remote.IP)
				}
			}
		}
	}()
	return ln, nil
}

func (ln *LBNetwork) HandleFunc(handler func([]byte, net.IP)) {
	ln.handler = append(ln.handler, handler)
}

func (ln *LBNetwork) Send(data []byte) error {
	_, err := ln.sender.Write(data)
	return err
}

type LBNetwork struct {
	config   LBNetworkConfig
	source   net.IP
	sender   *net.UDPConn
	receiver *net.UDPConn
	handler  []func([]byte, net.IP)
}
