package main

import (
	"crypto/tls"
	"flag"
	"github.com/lucas-clemente/quic-go"
	"io"
	"log"
	"os"
)

func main() {

	keyLogFile := flag.String("keylog", "", "key log file")
	ip := flag.String("ip", "localhost:4242", "IP:Port Address")
	//verbose := flag.Bool("v", false, "verbose (QUIC detailed logs)")
	//priorities := flag.String("prior","1 1 1","Stream priorities")
	scheduler := flag.String("scheduler","wfq","Scheduler type-> abs: absolute priorities, rr: roundrobin, wfq:weighted fair queue")
	flag.Parse()

	prior := []int64{5, 7, 1}

	config := &quic.Config{
		StreamPrior:					prior,
		TypePrior:                        *scheduler,
	}

	// QUIC logger

	var keyLog io.Writer
	if len(*keyLogFile) > 0 {
		f, err := os.Create(*keyLogFile)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		keyLog = f
	}

	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		KeyLogWriter:       keyLog,
		NextProtos:   []string{"quic-echo-example"},
	}

	//session, err := quic.DialAddr(*ip, tlsConf, config)
	session, err := quic.DialAddr(*ip,tlsConf, config)
	if err != nil {
		panic(err)
	}

	stream1,err1 := session.OpenStream()
	stream2,err2:= session.OpenStream()
	stream3,err3 := session.OpenStream()
	if err1 != nil || err2 != nil || err3 != nil {
		print("Error")
		return
	}

	msg1, err1 := stream1.Write([]byte("Hola"))
	msg2, err2 := stream2.Write([]byte("Que tal"))
	msg3, err3 := stream3.Write([]byte("?"))
	if err1 != nil || err2 != nil || err3 != nil {
		print("Error")
		return
	}
	print("\nMensaje stream1: %s", msg1,"\nMensaje stream2: %s", msg2, "\nMensaje stream3: %s", msg3)
}
