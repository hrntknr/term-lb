package main

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

type state int

const (
	StateEstablish = 1
	StateMigration = 2
)

type Connection struct {
	State state
	IP    net.IP
	Port  uint16
}

func newTCPHook(pcapIf string, port uint16) (*TCPHook, error) {
	handle, err := pcap.OpenLive(pcapIf, 0xffff, true, pcap.BlockForever)
	if err != nil {
		return nil, err
	}
	connMutex := new(sync.Mutex)

	hook := &TCPHook{
		connections: map[string]Connection{},
		connMutex:   connMutex,
	}

	go func() {
		defer handle.Close()
		packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
		for packet := range packetSource.Packets() {
			tcpPacket, ok := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
			if !ok {
				continue
			}
			if tcpPacket.SYN {
				continue
			}
			if uint16(tcpPacket.DstPort) != port {
				continue
			}
			ipPacket, ok := packet.Layer(layers.LayerTypeIPv6).(*layers.IPv6)
			if !ok {
				continue
			}
			hook.connMutex.Lock()
			_, ok = hook.connections[fmt.Sprintf("[%s]:%d", ipPacket.SrcIP, tcpPacket.SrcPort)]
			hook.connMutex.Unlock()
			if ok {
				continue
			}

			log.Printf("unknown: [%s]:%d\n", ipPacket.SrcIP, tcpPacket.SrcPort)
			hook.connMutex.Lock()
			hook.connections[fmt.Sprintf("[%s]:%d", ipPacket.SrcIP, tcpPacket.SrcPort)] = Connection{
				State: StateMigration,
				IP:    ipPacket.SrcIP,
				Port:  uint16(tcpPacket.SrcPort),
			}
			hook.connMutex.Unlock()
			for _, handler := range hook.handler {
				handler(ipPacket.SrcIP, uint16(tcpPacket.SrcPort))
			}
		}
	}()
	return hook, nil
}

func (hook *TCPHook) HandleFunc(handler func(net.IP, uint16)) {
	hook.handler = append(hook.handler, handler)
}

func (hook *TCPHook) AcceptEvent(ip net.IP, port uint16) {
	fmt.Printf("accept: [%s]:%d\n", ip, port)
	hook.connMutex.Lock()
	hook.connections[fmt.Sprintf("[%s]:%d", ip, port)] = Connection{
		State: StateEstablish,
		Port:  port,
		IP:    ip,
	}
	hook.connMutex.Unlock()
}
func (hook *TCPHook) CloseEvent(ip net.IP, port uint16, margin time.Duration) {
	go func() {
		time.Sleep(margin)
		hook.connMutex.Lock()
		delete(hook.connections, fmt.Sprintf("[%s]:%d", ip, port))
		hook.connMutex.Unlock()
	}()
}

type TCPHook struct {
	connections map[string]Connection
	connMutex   *sync.Mutex
	handler     []func(net.IP, uint16)
}
