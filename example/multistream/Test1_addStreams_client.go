package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"github.com/lucas-clemente/quic-go"
	"io"
	"log"
	"math/rand"
	"os"
	"sync"
)

func streamOpener (session quic.Connection, msg []byte) {
		stream, err := session.OpenStream()
		if (err != nil){
			panic("Error opening stream")
		}
		infoStream, err:= stream.Write(msg)
		fmt.Println(infoStream)
		if (err != nil){
			panic("Error sending info")
		}
}

func main() {

	keyLogFile := flag.String("keylog", "", "key log file")
	ip := flag.String("ip", "localhost:4242", "IP:Port Address")
	numStreams := flag.Int("nStream",1,"Number of streams that the client wants to open")
	mb := flag.Int("mb", 1, "File size in MiB")
	//verbose := flag.Bool("v", false, "verbose (QUIC detailed logs)")
	//priorities := flag.String("prior","1 1 1","Stream priorities")
	scheduler := flag.String("scheduler","rr","Scheduler type-> abs: absolute priorities, rr: roundrobin, wfq:weighted fair queue")
	flag.Parse()
	var wg sync.WaitGroup

	//QUIC config
	config := &quic.Config{
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
	//TLS config
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		KeyLogWriter:       keyLog,
		NextProtos:   []string{"quic-echo-example"},
	}

	//QUIC session
	session, err := quic.DialAddr(*ip,tlsConf, config)
	if err != nil{
		panic("Failed to create QUIC session")
	}


	// Create the message to send
	maxSendBytes :=  (*mb)*1024*1024
	lenStream:=maxSendBytes/(*numStreams)
	aux := 0
	//QUIC open streams
	for i:=0; i<*numStreams;i++{
		wg.Add(1)
		go func() {
			//defer wg.Done()
			message := make([]byte, lenStream) // Generate a message of PACKET_SIZE full of random information
			if n, err := rand.Read(message); err != nil {
				panic(fmt.Sprintf("Failed to create test message: wrote %d/%d Bytes; %v\n", n, maxSendBytes, err))
			}
			streamOpener(session, message)
			defer wg.Done()
		}()
		wg.Wait()
		aux += lenStream
	}

}