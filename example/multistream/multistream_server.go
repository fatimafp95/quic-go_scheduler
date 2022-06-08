package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/lucas-clemente/quic-go"
)

func main() {

	// Listen on the given network address for QUIC connection
	ip := flag.String("ip", "localhost:4242", "IP:Port Address")
	sess_chann := make(chan quic.Session)

	listener, err := quic.ListenAddr(*ip, nil, nil)
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			sess, err := listener.Accept(context.Background())
			if err != nil {
				fmt.Println("Error accepting: ", err.Error())
				return
			}
			sess_chann <- sess
		}
	}()
	defer listener.Close()
	for{
		select{
		case sess := <- sess_chann: //QUIC SESSION
			fmt.Println("Established QUIC connection")
			stream, err := sess.AcceptStream(context.Background())
			if err != nil{
				panic(err)
			}
			print(stream)
		}
	}
}

