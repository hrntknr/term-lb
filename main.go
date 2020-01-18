package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"

	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v2"
)

var configPath = flag.String("c", "./config.yml", "path of configuration file")

type Config struct {
	Backends  []Backend       `yaml:"backends"`
	LBNetwork LBNetworkConfig `yaml:"lbNetwork"`
}

type Backend struct {
	Hosts  []net.IP `yaml:"hosts"`
	Port   uint16   `yaml:"port"`
	Listen uint16   `yaml:"listen"`
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
			lb := newLB(backend, currentConfig)
			err := lb.startListen()
			if err != nil {
				log.Printf("error: %s\n", err)
			}
			wg.Done()
		}(backend)
	}
	wg.Wait()
}

func newLB(backend Backend, config Config) *lb {
	return &lb{
		backend: backend,
		config:  config,
	}
}

type lb struct {
	backend Backend
	config  Config
}

func (lb *lb) startListen() error {
	fd, err := unix.Socket(unix.AF_INET6, unix.SOCK_STREAM, unix.IPPROTO_TCP)
	if err != nil {
		return err
	}
	if err := unix.Bind(fd, &unix.SockaddrInet6{
		Addr: [16]byte{},
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
	lbnet.HandleFunc(func(buf []byte, remote net.IP) {
	})

	go func() {
		for {
			nfd, _, err := unix.Accept(fd)
			if err != nil {
				log.Printf("error: %s\n", err)
			}
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

				exit := make(chan int, 1)

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
			}()
		}
	}()
	<-quit
	unix.Close(fd)
	return nil
}
