package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	if len(os.Args) != 2 {
		panic(fmt.Errorf("invalid arg length"))
	}
	n, err := strconv.Atoi(os.Args[1])
	if err != nil {
		panic(err)
	}

	c := make(chan int)

	for i := 0; i < n; i++ {
		ch, err := newConnection()
		if err != nil {
			panic(err)
		}
		fmt.Println(i)
		go func(i int) {
			for {
				<-c
				ch <- 0
			}
		}(i)
		time.Sleep(10 * time.Millisecond)
	}

	// for range time.Tick(time.Second) {
	// 	for i := 0; i < n; i++ {
	// 		c <- 0
	// 	}
	// }
	for {
		bufio.NewReader(os.Stdin).ReadString('\n')
		for i := 0; i < n; i++ {
			c <- 0
		}
	}
}

func newConnection() (chan int, error) {
	conn, err := net.Dial("tcp", "[fc01::1]:8080")
	if err != nil {
		return nil, err
	}

	ch := make(chan int)

	go func() {
		defer conn.Close()

		exit := make(chan int)

		queue := map[string]time.Time{}

		go func() {
			for seq := 0; true; seq++ {
				<-ch
				payload := fmt.Sprintf("%d", seq)
				queue[payload] = time.Now()
				_, err := conn.Write([]byte(payload + ";"))
				if err != nil {
					log.Printf("%s\n", err)
					exit <- 0
				}
			}
		}()
		go func() {
			buf := make([]byte, 9000)
			for {
				n, err := conn.Read(buf)
				if err != nil {
					log.Printf("%s\n", err)
					exit <- 0
				}
				payloads := strings.Split(string(buf[:n]), ";")
				for _, payload := range payloads {
					if payload == "" {
						continue
					}
					start := queue[payload]
					fmt.Printf("%s %d\n", payload, time.Now().Sub(start)/time.Millisecond)
				}
			}
		}()

		<-exit
	}()

	return ch, nil
}
