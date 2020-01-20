package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
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
	Hosts     []net.IP `yaml:"hosts"`
	Port      uint16   `yaml:"port"`
	Listen    uint16   `yaml:"listen"`
	Vip       net.IP   `yaml:"vip"`
	Interface string   `yaml:"interface"`
}

type LBNetworkConfig struct {
	Network string `yaml:"network"`
	Source  string `yaml:"source"`
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

			lb := newLB(backend, currentConfig, hook)
			if err := lb.startListen(); err != nil {
				log.Printf("error: %s\n", err)
			}
			wg.Done()
		}(backend)
	}
	wg.Wait()
}

func newLB(backend Backend, config Config, hook *TCPHook) *lb {
	return &lb{
		backend: backend,
		config:  config,
		hook:    hook,
	}
}

type lb struct {
	backend Backend
	config  Config
	hook    *TCPHook
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
	lbnet.HandleFunc(func(buf []byte, remote net.IP) {
		log.Printf("rcv lbnet: %s\n", buf)
		commands := strings.Split(string(buf), " ")
		if len(commands) == 2 {
			ch, ok := rcvLBNet[fmt.Sprintf("[%s]:%s", commands[0], commands[1])]
			if ok {
				ch <- 0
			} else {
				log.Println("not found connection")
			}
		}
		if len(commands) == 3 {
			log.Println("restore connection")
			addr := net.ParseIP(commands[0])
			port, err := strconv.Atoi(commands[1])
			if err != nil {
				log.Printf("error: %s\n", err)
				return
			}
			repair := TCPRepair{}
			if err := json.Unmarshal([]byte(commands[2]), &repair); err != nil {
				log.Printf("error: %s\n", err)
				return
			}

			nfd, err := unix.Socket(unix.AF_INET6, unix.SOCK_STREAM, 0)
			if err != nil {
				log.Printf("error: %s\n", err)
				return
			}
			if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR, 1); err != nil {
				log.Printf("error: %s\n", err)
				return
			}
			if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR_QUEUE, TCP_SEND_QUEUE); err != nil {
				log.Printf("error: %s\n", err)
				return
			}
			if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_QUEUE_SEQ, repair.SndSeq); err != nil {
				log.Printf("error: %s\n", err)
				return
			}
			if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR_QUEUE, TCP_RECV_QUEUE); err != nil {
				log.Printf("error: %s\n", err)
				return
			}
			if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_QUEUE_SEQ, repair.RcvSeq); err != nil {
				log.Printf("error: %s\n", err)
				return
			}
			// var ip [16]byte
			// copy(ip[:], lb.backend.Vip)
			// if err := unix.Bind(fd, &unix.SockaddrInet6{
			// 	Addr: ip,
			// 	Port: int(lb.backend.Listen),
			// }); err != nil {
			// 	log.Printf("bind error: %s\n", err)
			// 	return
			// }
			if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_QUEUE_SEQ, repair.RcvSeq); err != nil {
				log.Printf("error: %s\n", err)
				return
			}
			if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_MAXSEG, repair.Mss); err != nil {
				log.Printf("error: %s\n", err)
				return
			}
			if err := SetsockoptTcpRepairWindow(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR_WINDOW, repair.Window); err != nil {
				log.Printf("error: %s\n", err)
				return
			}
			var ip [16]byte
			copy(ip[:], lb.backend.Vip)
			if err := unix.Bind(nfd, &unix.SockaddrInet6{
				Addr: ip,
				Port: int(lb.backend.Listen),
			}); err != nil {
				log.Printf("bind error: %s\n", err)
				return
			}
			var addrByte [16]byte
			copy(addrByte[:], addr)
			if err := unix.Connect(nfd, &unix.SockaddrInet6{
				Port: port,
				Addr: addrByte,
			}); err != nil {
				log.Printf("connect error: %s\n", err)
				return
			}
			if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR, 0); err != nil {
				log.Printf("error10: %s\n", err)
				return
			}
			go func() {
				buf := make([]byte, 9000)
				for {
					n, err := unix.Read(nfd, buf)
					if err != nil {
						log.Printf("error: %s\n", err)
						return
					}
					if n == 0 {
						return
					}
					if _, err := unix.Write(nfd, buf[:n]); err != nil {
						log.Printf("error: %s\n", err)
						return
					}
					fmt.Printf("%s", buf[:n])
				}
			}()
		}
	})
	lb.hook.HandleFunc(func(ip net.IP, port uint16) {
		log.Printf("search lbnet: [%s]:%d\n", ip, port)
		lbnet.Send([]byte(fmt.Sprintf("%s %d", ip, port)))
	})

	go func() {
		for {
			nfd, sa, err := unix.Accept(fd)
			if err != nil {
				log.Printf("error: %s\n", err)
			}
			exit := make(chan int, 1)
			sa6 := sa.(*unix.SockaddrInet6)
			ip := net.IP(sa6.Addr[:])
			lb.hook.AcceptEvent(ip, uint16(sa6.Port))
			rcvLBNet[fmt.Sprintf("[%s]:%d", ip, sa6.Port)] = make(chan int)
			go func() {
				<-rcvLBNet[fmt.Sprintf("[%s]:%d", ip, sa6.Port)]
				err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR, 1)
				if err != nil {
					log.Printf("error: %s\n", err)
					return
				}
				window, err := GetsockoptTcpRepairWindow(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR_WINDOW)
				if err != nil {
					log.Printf("error: %s\n", err)
					return
				}
				fmt.Printf("window: %v\n", window)
				mss, err := unix.GetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_MAXSEG)
				if err != nil {
					log.Printf("error: %s\n", err)
					return
				}
				fmt.Printf("mss: %d\n", mss)
				if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR_QUEUE, TCP_SEND_QUEUE); err != nil {
					log.Printf("error: %s\n", err)
					return
				}
				sndSeq, err := unix.GetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_QUEUE_SEQ)
				if err != nil {
					if err != nil {
						log.Printf("error: %s\n", err)
						return
					}
				}
				fmt.Printf("sndSeq: %d\n", sndSeq)
				if err := unix.SetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_REPAIR_QUEUE, TCP_RECV_QUEUE); err != nil {
					log.Printf("error: %s\n", err)
					return
				}
				rcvSeq, err := unix.GetsockoptInt(nfd, unix.IPPROTO_TCP, unix.TCP_QUEUE_SEQ)
				if err != nil {
					if err != nil {
						log.Printf("error: %s\n", err)
						return
					}
				}
				fmt.Printf("rcvSeq: %d\n", rcvSeq)
				log.Printf("TCP_REPAIR success")
				repairJson, err := json.Marshal(TCPRepair{
					Window: window,
					Mss:    mss,
					RcvSeq: rcvSeq,
					SndSeq: sndSeq,
				})
				if err != nil {
					log.Printf("error: %s\n", err)
				}
				lbnet.Send([]byte(fmt.Sprintf("%s %d %s", ip, sa6.Port, repairJson)))
			}()

			go func() {
				defer unix.Close(nfd)
				cfd, err := unix.Socket(unix.AF_INET6, unix.SOCK_STREAM, unix.IPPROTO_TCP)
				if err != nil {
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

				go func() {
					buf := make([]byte, 9000)
					for {
						n, err := unix.Read(nfd, buf)
						if err != nil {
							log.Printf("error: %s\n", err)
							exit <- 1
							return
						}
						if n == 0 {
							exit <- 1
							return
						}
						if _, err := unix.Write(cfd, buf[:n]); err != nil {
							log.Printf("error: %s\n", err)
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
							log.Printf("error: %s\n", err)
							exit <- 1
							return
						}
						if n == 0 {
							exit <- 1
							return
						}
						if _, err := unix.Write(nfd, buf[:n]); err != nil {
							log.Printf("error: %s\n", err)
							exit <- 1
							return
						}
					}
				}()

				<-exit
				lb.hook.CloseEvent(ip, uint16(sa6.Port), 1*time.Second)
			}()
		}
	}()
	<-quit
	unix.Close(fd)
	return nil
}
