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
	"time"
)

var t trace
type trace struct{
	fileName string
	file *os.File
	timeStart time.Time
}

func (t *trace) Print(tx_time int64, fileName string) {
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		t.file, _ = os.OpenFile(fileName, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
		fmt.Fprintf(t.file, " TX TIME \n")
	} else {
		t.file, _ = os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	}

	fmt.Fprintf(t.file,"%f\n",tx_time)
	t.file.Close()
}

func streamOpener (session quic.Connection, msg []byte, i int, fileName string) {
	stream, err := session.OpenStream()
	if (err != nil){
		panic("Error opening stream")
	}
	if i==1 {
		start := time.Now().UnixNano()
		t.Print(start, fileName) //TimeStamp for the initial stream
		fmt.Println("Timestamp start:",start)
	}
	infoStream, err:= stream.Write(msg)
	fmt.Println("Client-Info:",infoStream)
	if (err != nil){
		panic("Error sending info")
	}
	time.Sleep(1*time.Second)
	if err = stream.Close(); err != nil {
		fmt.Println("Failed to close the stream")
	}
}

func main() {

	keyLogFile := flag.String("keylog", "", "key log file")
	ip := flag.String("ip", "localhost:4242", "IP:Port Address")
	numStreams := flag.Int("ns",1,"Number of streams that the client wants to open")
	mb := flag.Int("mb", 1, "File size in MiB")
	//verbose := flag.Bool("v", false, "verbose (QUIC detailed logs)")
	//priorities := flag.String("prior","1 1 1","Stream priorities")
	scheduler := flag.String("scheduler","rr","Scheduler type-> abs: absolute priorities, rr: roundrobin, wfq:weighted fair queue")
	fileName := flag.String("file","","Files name")

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

	//time.Sleep(5*time.Second)

	// Create the message to send
	maxSendBytes :=  (*mb)*1024*1024
	lenStream:=maxSendBytes/(*numStreams)

	//QUIC open streams
	numThreads := *numStreams
	wg.Add(numThreads)
	for i:=1; i<=*numStreams;i++{
		go func() {
			defer wg.Done()
			message := make([]byte, lenStream) // Generate a message of PACKET_SIZE full of random information
			if n, err := rand.Read(message); err != nil {
				panic(fmt.Sprintf("Failed to create test message: wrote %d/%d Bytes; %v\n", n, maxSendBytes, err))
			}
			streamOpener(session, message, i, *fileName)
		}()
		wg.Wait()
	}

	// Close stream and connection
	var errMsg string

	if err = session.CloseWithError(0, ""); err != nil {
		errMsg += "; SessionErr: " + err.Error()
	}
	if errMsg != "" {
		fmt.Println("Client: Connection closed with errors" + errMsg)
	} else {
		fmt.Println("Client: Connection closed")
	}

}