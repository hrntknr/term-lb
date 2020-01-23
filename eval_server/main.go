package main

import (
	"net"
)

func main() {
	err := _main()
	if err != nil {
		panic(err)
	}
}

func _main() error {
	listen, err := net.Listen("tcp", "[::]:8080")
	if err != nil {
		return err
	}
	for {
		conn, err := listen.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer conn.Close()
			for {
				buf := make([]byte, 9000)
				n, err := conn.Read(buf)
				if err != nil {
					return
				}
				_, err = conn.Write(buf[:n])
				if err != nil {
					return
				}
			}
		}()
	}
}
