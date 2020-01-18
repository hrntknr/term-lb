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
	sender, err := net.DialUDP("udp", nil, address)
	if err != nil {
		return nil, err
	}
	receiver, err := net.ListenMulticastUDP("udp", nil, address)
	if err != nil {
		return nil, err
	}
	return &LBNetwork{
		sender:   sender,
		receiver: receiver,
	}, nil
}

func (ln *LBNetwork) HandleFunc(handler func([]byte, net.IP)) {
	go func() {
		buffer := make([]byte, 1500)
		for {
			n, remote, err := ln.receiver.ReadFromUDP(buffer)
			if err != nil {
				log.Printf("error: %s\n", err)
			}
			handler(buffer[:n], remote.IP)
		}
	}()
}

func (ln *LBNetwork) Send(data []byte) error {
	_, err := ln.sender.Write(data)
	return err
}

type LBNetwork struct {
	sender   *net.UDPConn
	receiver *net.UDPConn
}
