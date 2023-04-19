package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"github.com/lucas-clemente/quic-go"

	//"math/rand"
	//	xrand "golang.org/x/exp/rand"

	//"github.com/lucas-clemente/quic-go/internal/utils"
	"io"
	"log"
	//"math"
	//"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	//	"gonum.org/v1/gonum/stat/distuv"
)


type traceClient struct {
	fileName  string
	file      *os.File
	timeStart time.Time
}

func NewTraceClient (fileName string) *traceClient{
	t := traceClient{fileName: fileName}
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		t.file, _ = os.OpenFile(fileName, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
		fmt.Fprintf(t.file, " TX TIME\tN\t\n")
	} else {
		t.file, _ = os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	}
	return &t
}
func (t *traceClient) PrintDroneClient(tx_time int64) {
	fmt.Fprintf(t.file, "%d\n", tx_time)
}
func streamOpener(session quic.Connection) quic.Stream {
	stream, err := session.OpenStream()
	//defer stream.Close()
	if err != nil {
		panic("Error opening stream")
	}
	stream.Write([]byte{'F'})
	fmt.Println("Stream opened with the ID:\n", stream.StreamID())
	return stream
}


//Drone traffic: Drone packet trace captured at the 5GENESIS Athens testbed. The trace consists of packets sent between the drone and a remote controller
//over a 5G connection. The traffic was captured on-board the drone PixHawk 4.02 autopilot. The autopilot exploited UE tethering in order to connect via
//a 5G radio interface with the 5G RAN. The traffic capture is comprised of MAVLink3 packets (over UDP) sent over the 5G RAN with navigation commands.

func main() {
	// Listen on the given network address for QUIC connection
	keyLogFile := flag.String("keylog", "keylog.log", "key log file")
	ip := flag.String("ip", "localhost:4242", "IP:Port Address")
	numPackets:=flag.Int("numPackets", 1, "Number of packets (not app. messages)")
	numStreams := flag.Int("ns", 1, "Number of streams to use")
	//mb := flag.Int("mb", 1, "File size in MiB")
	fileNameBulk := flag.String("fileBulk", "", "Files name")
	scheduler := flag.String("scheduler", "rr", "Scheduler type: rr=Round Robin, wfq=Weight Fair queueing, abs=Absolute Priorization")
	order := flag.String("order", "1", "Weight or position to process each stream.")
	flag.Parse()
	var wg sync.WaitGroup


	//Creation of the priorizaton slice
	splitString := strings.Split(*order, "")
	lenPrior := len(*order)
	slice := make([]int, lenPrior)
	for i := 0; i < lenPrior; i++ {
		value, err := strconv.Atoi(splitString[i])
		if err != nil {
			fmt.Println("Error creating the int slice: ", err)
		}
		slice[i] = value
	}

	//QUIC config
	quicConfig := &quic.Config{
		DisablePathMTUDiscovery: true,
		TypePrio:               *scheduler,
		StreamPrio:             slice,
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
		NextProtos:         []string{"quic-echo-example"},
	}

	//QUIC session
	session, err := quic.DialAddr(*ip, tlsConf, quicConfig)
	defer session.CloseWithError(0, "")
	if err != nil {
		panic("Failed to create QUIC session")
	} else {
		fmt.Println("QUIC session created\n")
	}

	//QUIC open streams
	numThreads := *numStreams
	var mutex sync.Mutex
	var stream [2]quic.Stream
	wg.Add(numThreads)
	for i := 0; i < numThreads; i++ {
		mutex.Lock()
		go func(i int) {
			defer wg.Done()
			stream[i] = streamOpener(session)
			mutex.Unlock()
		}(i)
	}
	wg.Wait()


	//Channel to communicate streams//
	stopChannel:= make(chan struct{})
	/////////////////////////////////

	// Create the BULK message to send (MB)
	lenMessages := (*numPackets)*1177
	miniBulk := make([]byte, lenMessages)
	numMessages:= 500 //Number of messages with miniBulk
	megaBulk := make([]byte, 10*1024*1024)
	//nBulk:=numThreads-1



	wg.Add(1)
	go func() {
		defer wg.Done()
		n, _ := stream[1].Write(megaBulk)
		select {
		case <-stopChannel:
			stream[1].CancelWrite(0)
		}
		fmt.Println("Total bytes sent from stream ID:", n, stream[1].StreamID())
	}()

	wg.Add(1)
	go func() {
		t:=NewTraceClient(*fileNameBulk)
		cumulative := 0
		defer wg.Done()
		for m:=0;m<numMessages;m++ {
			stamp := time.Now() // print the timestamp
			t.PrintDroneClient(stamp.UnixNano())
			n, _ := stream[0].Write(miniBulk)
			cumulative += n
			fmt.Println("Amount of bytes sent in the message:", n)
			time.Sleep(2*40*time.Millisecond)
		}
		fmt.Println("Total bytes sent from stream ID:", cumulative, stream[0].StreamID())
		time.Sleep(5*time.Second)
		close(stopChannel)
		t.file.Close()
	}()

	time.Sleep(5*time.Second)
	wg.Wait()

}
