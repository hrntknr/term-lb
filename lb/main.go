package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v2"
)

var configPath = flag.String("c", "./config.yml", "path of configuration file")

type Config struct {
	Backends  []Backend       `yaml:"backends"`
	LBNetwork LBNetworkConfig `yaml:"lbNetwork"`
}

type Backend struct {
	Hosts        []net.IP `yaml:"hosts"`
	Port         uint16   `yaml:"port"`
	Listen       uint16   `yaml:"listen"`
	Vip          net.IP   `yaml:"vip"`
	Interface    string   `yaml:"interface"`
	AddressRange string   `yaml:"addressRange"`
}

type LBNetworkConfig struct {
	Network  string        `yaml:"network"`
	Source   string        `yaml:"source"`
	Commands CommandConfig `yaml:"commands"`
}

type CommandConfig struct {
	Active  string `yaml:"active"`
	Standby string `yaml:"standby"`
}

func main() {
	flag.Usage = func() {
		flag.PrintDefaults()
	}
	flag.Parse()
	buf, err := ioutil.ReadFile(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	var currentConfig Config
	if err := yaml.Unmarshal(buf, &currentConfig); err != nil {
		log.Fatal(err)
	}

	wg := &sync.WaitGroup{}
	for _, backend := range currentConfig.Backends {
		wg.Add(1)
		go func(backend Backend) {
			hook, err := newTCPHook(backend.Interface, backend.Port)
			if err != nil {
				log.Printf("error: %s\n", err)
				return
			}

			lb, err := newLB(backend, currentConfig, hook)
			if err != nil {
				log.Printf("error: %s\n", err)
			}
			if err := lb.startListen(); err != nil {
				log.Printf("error: %s\n", err)
			}
			wg.Done()
		}(backend)
	}
	wg.Wait()
}

func newLB(backend Backend, config Config, hook *TCPHook) (*lb, error) {
	addrManager, err := newAddrManager(backend.AddressRange)
	if err != nil {
		return nil, err
	}
	return &lb{
		backend:     backend,
		config:      config,
		hook:        hook,
		addrManager: addrManager,
	}, nil
}

type lb struct {
	backend     Backend
	config      Config
	hook        *TCPHook
	addrManager *AddrManager
}

func (lb *lb) startListen() error {
	fd, err := unix.Socket(unix.AF_INET6, unix.SOCK_STREAM, 0)
	if err != nil {
		return err
	}
	var ip [16]byte
	copy(ip[:], lb.backend.Vip)
	if err := unix.Bind(fd, &unix.SockaddrInet6{
		Addr: ip,
		Port: int(lb.backend.Listen),
	}); err != nil {
		return err
	}
	if err := unix.Listen(fd, 1); err != nil {
		return err
	}
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)

	lbnet, err := newLBNetwork(lb.config.LBNetwork)
	if err != nil {
		return err
	}
	rcvLBNet := map[string]chan int{}
	rcvLBNetMutex := new(sync.Mutex)
	lbnet.HandleFunc(func(buf []byte, remote net.IP) {
		log.Printf("rcv lbnet: %s\n", buf)
		commands := strings.Split(string(buf), " ")
		if len(commands) == 2 {
			rcvLBNetMutex.Lock()
			ch, ok := rcvLBNet[fmt.Sprintf("[%s]:%s", commands[0], commands[1])]
			rcvLBNetMutex.Unlock()
			if ok {
				ch <- 0
			} else {
				log.Println("not found connection")
			}
		}
		if len(commands) == 4 {
			log.Println("restore connection")
			addr := net.ParseIP(commands[0])
			port, err := strconv.Atoi(commands[1])
			if err != nil {
				log.Printf("error: %s\n", err)
				return
			}
			repairUpstream := TCPRepair{}
			if err := json.Unmarshal([]byte(commands[2]), &repairUpstream); err != nil {
				log.Printf("error: %s\n", err)
				return
			}

			nfd, err := lb.repair(lb.backend.Vip, lb.backend.Listen, addr, uint16(port), repairUpstream, false)
			if err != nil {
				log.Printf("error: %s\n", err)
				return
			}
			exit := make(chan int, 1)
			repairDownstream := TCPRepair{}
			if err := json.Unmarshal([]byte(commands[3]), &repairDownstream); err != nil {
				log.Printf("error: %s\n", err)
				return
			}
			cfd, err := lb.repair(repairDownstream.Saddr, repairDownstream.Sport, repairDownstream.Daddr, repairDownstream.Dport, repairDownstream, true)
			if err != nil {
				log.Printf("error: %s\n", err)
				return
			}
			defer unix.Close(cfd)
			defer unix.Close(nfd)
			go func() {
				cmd := fmt.Sprintf(lb.config.LBNetwork.Commands.Active, repairDownstream.Saddr)
				log.Printf("exec: %s\n", cmd)
				err := exec.Command("sh", "-c", cmd).Run()
				if err != nil {
					log.Printf("warn: %s\n", err)
				}
			}()

			lb.pipe(nfd, cfd, exit)

			go func() {
				ch := make(chan int)
				rcvLBNetMutex.Lock()
				rcvLBNet[fmt.Sprintf("[%s]:%d", addr, port)] = ch
				rcvLBNetMutex.Unlock()
				defer unix.Close(cfd)
				defer unix.Close(nfd)
				<-ch
				go func() {
					cmd := fmt.Sprintf(lb.config.LBNetwork.Commands.Standby, repairDownstream.Saddr)
					log.Printf("exec: %s\n", cmd)
					err := exec.Command("sh", "-c", cmd).Run()
					if err != nil {
						log.Printf("error: %s\n", err)
					}
				}()
				upstream, downstream, err := lb.createRepairInfo(nfd, cfd)
				if err != nil {
					log.Printf("error: %s\n", err)
					return
				}
				upstreamJSON, err := json.Marshal(upstream)
				if err != nil {
					log.Printf("error: %s\n", err)
					return
				}
				downstreamJSON, err := json.Marshal(downstream)
				if err != nil {
					log.Printf("error: %s\n", err)
					return
				}
				lbnet.Send([]byte(fmt.Sprintf("%s %d %s %s", addr, port, upstreamJSON, downstreamJSON)))
				rcvLBNetMutex.Lock()
				delete(rcvLBNet, fmt.Sprintf("[%s]:%d", addr, port))
				rcvLBNetMutex.Unlock()
				lb.hook.CloseEvent(addr, uint16(port), time.Second)
			}()

			<-exit
		}
	})
	lb.hook.HandleFunc(func(ip net.IP, port uint16) {
		log.Printf("search lbnet: [%s]:%d\n", ip, port)
		err := lbnet.Send([]byte(fmt.Sprintf("%s %d", ip, port)))
		if err != nil {
			log.Printf("error: %s\n", err)
		}
	})

	for i := 0; i < 100; i++ {
		go func() {
			for {
				nfd, sa, err := unix.Accept(fd)
				if err != nil {
					log.Printf("error: %s\n", err)
				}
				exit := make(chan int, 1)
				sa6 := sa.(*unix.SockaddrInet6)
				ip := net.IP(sa6.Addr[:])
				go lb.hook.AcceptEvent(ip, uint16(sa6.Port))

				go func() {
					defer unix.Close(nfd)
					cfd, err := unix.Socket(unix.AF_INET6, unix.SOCK_STREAM, unix.IPPROTO_TCP)
					if err != nil {
						log.Printf("error: %s\n", err)
						return
					}
					if err := unix.SetsockoptInt(cfd, unix.SOL_IP, unix.IP_FREEBIND, 1); err != nil {
						log.Printf("error: %s\n", err)
						return
					}
					laddr, err := lb.addrManager.releaseIP()
					if err != nil {
						log.Printf("error: %s\n", err)
						return
					}
					log.Printf("use %s to downstream\n", laddr)
					var addr [16]byte
					copy(addr[:], laddr)
					if err := unix.Bind(cfd, &unix.SockaddrInet6{
						Addr: addr,
					}); err != nil {
						log.Printf("error: %s\n", err)
						return
					}
					var host [16]byte
					copy(host[:], lb.backend.Hosts[0])
					if err := unix.Connect(cfd, &unix.SockaddrInet6{
						Addr: host,
						Port: int(lb.backend.Port),
					}); err != nil {
						log.Printf("error: %s\n", err)
						return
					}
					defer unix.Close(cfd)
					lb.pipe(nfd, cfd, exit)

					go func() {
						ch := make(chan int)
						rcvLBNetMutex.Lock()
						rcvLBNet[fmt.Sprintf("[%s]:%d", ip, sa6.Port)] = ch
						rcvLBNetMutex.Unlock()
						defer unix.Close(cfd)
						defer unix.Close(nfd)
						<-ch
						log.Printf("rcv ch: [%s]:%d", ip, sa6.Port)
						upstream, downstream, err := lb.createRepairInfo(nfd, cfd)
						if err != nil {
							log.Printf("error: %s\n", err)
							return
						}
						upstreamJSON, err := json.Marshal(upstream)
						if err != nil {
							log.Printf("error: %s\n", err)
							return
						}
						downstreamJSON, err := json.Marshal(downstream)
						if err != nil {
							log.Printf("error: %s\n", err)
							return
						}
						lbnet.Send([]byte(fmt.Sprintf("%s %d %s %s", ip, sa6.Port, upstreamJSON, downstreamJSON)))
						rcvLBNetMutex.Lock()
						delete(rcvLBNet, fmt.Sprintf("[%s]:%d", ip, sa6.Port))
						rcvLBNetMutex.Unlock()
						lb.hook.CloseEvent(ip, uint16(sa6.Port), time.Second)
					}()

					<-exit
					lb.hook.CloseEvent(ip, uint16(sa6.Port), 1*time.Second)
				}()
			}
		}()
	}
	<-quit
	unix.Close(fd)
	return nil
}

func (lb *lb) destroy(nfd int) (TCPRepair, error) {

	err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR, 1)
	if err != nil {
		return TCPRepair{}, nil
	}
	window, err := GetsockoptTcpRepairWindow(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR_WINDOW)
	if err != nil {
		return TCPRepair{}, nil
	}
	mss, err := unix.GetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_MAXSEG)
	if err != nil {
		return TCPRepair{}, nil
	}
	if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR_QUEUE, TCP_SEND_QUEUE); err != nil {
		return TCPRepair{}, nil
	}
	sndSeq, err := unix.GetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_QUEUE_SEQ)
	if err != nil {
		return TCPRepair{}, nil
	}
	if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR_QUEUE, TCP_RECV_QUEUE); err != nil {
		return TCPRepair{}, nil
	}
	rcvSeq, err := unix.GetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_QUEUE_SEQ)
	if err != nil {
		if err != nil {
			return TCPRepair{}, nil
		}
	}
	repair := TCPRepair{
		Window: window,
		Mss:    mss,
		RcvSeq: rcvSeq,
		SndSeq: sndSeq,
	}
	return repair, nil
}

func (lb *lb) repair(saddr net.IP, sport uint16, daddr net.IP, dport uint16, repair TCPRepair, anyIP bool) (int, error) {
	nfd, err := unix.Socket(unix.AF_INET6, unix.SOCK_STREAM, 0)
	if err != nil {
		return 0, err
	}
	if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR, 1); err != nil {
		return 0, err
	}
	if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR_QUEUE, TCP_SEND_QUEUE); err != nil {
		return 0, err
	}
	if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_QUEUE_SEQ, repair.SndSeq); err != nil {
		return 0, err
	}
	if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR_QUEUE, TCP_RECV_QUEUE); err != nil {
		return 0, err
	}
	if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_QUEUE_SEQ, repair.RcvSeq); err != nil {
		return 0, err
	}
	if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_QUEUE_SEQ, repair.RcvSeq); err != nil {
		return 0, err
	}
	if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_MAXSEG, repair.Mss); err != nil {
		return 0, err
	}
	if err := SetsockoptTcpRepairWindow(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR_WINDOW, repair.Window); err != nil {
		return 0, err
	}
	if anyIP {
		if err := unix.SetsockoptInt(nfd, unix.SOL_IP, unix.IP_FREEBIND, 1); err != nil {
			return 0, err
		}
	}
	var ip [16]byte
	copy(ip[:], saddr)
	if err := unix.Bind(nfd, &unix.SockaddrInet6{
		Addr: ip,
		Port: int(sport),
	}); err != nil {
		return 0, err
	}
	var addrByte [16]byte
	copy(addrByte[:], daddr)
	if err := unix.Connect(nfd, &unix.SockaddrInet6{
		Port: int(dport),
		Addr: addrByte,
	}); err != nil {
		return 0, err
	}
	if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR, 0); err != nil {
		return 0, err
	}
	return nfd, nil
}

func (lb *lb) pipe(nfd int, cfd int, exit chan int) {
	go func() {
		buf := make([]byte, 9000)
		for {
			n, err := unix.Read(nfd, buf)
			if err != nil {
				log.Printf("error1.1: %s\n", err)
				exit <- 1
				return
			}
			log.Printf("rcv1: %d\n", n)
			if n == 0 {
				exit <- 1
				return
			}
			if _, err := unix.Write(cfd, buf[:n]); err != nil {
				log.Printf("error1.2: %s\n", err)
				exit <- 1
				return
			}
		}
	}()

	go func() {
		buf := make([]byte, 9000)
		for {
			n, err := unix.Read(cfd, buf)
			if err != nil {
				log.Printf("error2.1: %s\n", err)
				exit <- 1
				return
			}
			log.Printf("rcv2: %d\n", n)
			if n == 0 {
				exit <- 1
				return
			}
			if _, err := unix.Write(nfd, buf[:n]); err != nil {
				log.Printf("error2.2: %s\n", err)
				exit <- 1
				return
			}
		}
	}()
}

func (lb *lb) createRepairInfo(nfd int, cfd int) (TCPRepair, TCPRepair, error) {

	repairUpstream, err := lb.destroy(nfd)
	if err != nil {
		return TCPRepair{}, TCPRepair{}, err
	}
	log.Printf("TCP_REPAIR success")

	sa, err := unix.Getsockname(cfd)
	if err != nil {
		return TCPRepair{}, TCPRepair{}, err
	}

	csa6, ok := sa.(*unix.SockaddrInet6)
	if !ok {
		return TCPRepair{}, TCPRepair{}, err
	}

	repairDownstream, err := lb.destroy(cfd)
	if err != nil {
		return TCPRepair{}, TCPRepair{}, err
	}
	repairDownstream.Saddr = net.IP(csa6.Addr[:])
	repairDownstream.Sport = uint16(csa6.Port)
	repairDownstream.Dport = lb.backend.Port
	repairDownstream.Daddr = lb.backend.Hosts[0]

	return repairUpstream, repairDownstream, nil
}
