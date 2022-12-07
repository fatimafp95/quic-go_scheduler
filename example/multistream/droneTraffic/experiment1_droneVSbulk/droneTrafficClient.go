package main

import (
	"crypto/tls"
	"encoding/csv"
	"flag"
	"fmt"
	"github.com/lucas-clemente/quic-go"
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
func (t *traceClient) PrintDroneClient(tx_time int64, aux int8, drone []byte) {
	fmt.Fprintf(t.file, "%d\t%d\t%d\n", tx_time,aux,drone)
}
func streamOpener(session quic.Connection) quic.Stream {
	stream, err := session.OpenStream()
	if err != nil {
		panic("Error opening stream")
	}
	stream.Write([]byte{'F'})
	fmt.Println("Stream opened with the ID:", stream.StreamID())
	return stream
}

//Drone traffic: Drone packet trace captured at the 5GENESIS Athens testbed. The trace consists of packets sent between the drone and a remote controller
//over a 5G connection. The traffic was captured on-board the drone PixHawk 4.02 autopilot. The autopilot exploited UE tethering in order to connect via
//a 5G radio interface with the 5G RAN. The traffic capture is comprised of MAVLink3 packets (over UDP) sent over the 5G RAN with navigation commands.

func main() {
	// Listen on the given network address for QUIC connection
	keyLogFile := flag.String("keylog", "", "key log file")
	ip := flag.String("ip", "localhost:4242", "IP:Port Address")
	numStreams := flag.Int("ns", 1, "Number of streams to use")
	mb := flag.Int("mb", 1, "File size in MiB")
	fileNameBulk := flag.String("fileBulk", "", "Files name")
	fileNameDrone := flag.String("fileDrone", "", "Files name")
	scheduler := flag.String("scheduler", "rr", "Scheduler type: rr=Round Robin, wfq=Weight Fair queueing, abs=Absolute Priorization")
	order := flag.String("order", "1", "Weight or position to process each stream.")
	flag.Parse()
	var wg sync.WaitGroup

	//Opening drone traces file
	csvFile, err := os.Open("drone_processed.csv")
	if err != nil {
		fmt.Println("Error opening the drone.csv file")
		panic(err)
	}
	//fmt.Println("Successfully Opened file")
	defer csvFile.Close()
	data, err := csv.NewReader(csvFile).ReadAll()
	if data == nil {
		fmt.Println("Error reading the file")
	}

	//Opening drone traces file
	var (
		temp    [163478]time.Duration
		dataLen [163478]int
	)
	for i, line := range data {
		temp[i], _ = time.ParseDuration(line[0])
		dataLen[i], _ = strconv.Atoi(line[1])
	}

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
		TypePrior:               *scheduler,
		StreamPrior:             slice,
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
	if err != nil {
		panic("Failed to create QUIC session")
	} else {
		fmt.Println("QUIC session created\n")
	}

	//QUIC open streams
	numThreads := *numStreams
	var mutex sync.Mutex
	var stream [3]quic.Stream
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
//	fmt.Println("Stream slice:", stream[0].StreamID(), stream[1].StreamID())


	/*** Test message ***/
	/*message2 := make([]byte,maxSendBytes)
	b := []byte("HOLA")
	message2 = append(message2,b...)
*/
	/**************DRONE MESSAGES******************/
	messageDroneProto := make([]byte, 242)
	var messageDrone []byte
	wg.Add(1)
	go func() {
		t:=NewTraceClient(*fileNameDrone)
		defer wg.Done()
		//fmt.Println("Drone file is reading")
		//stream[0].Write(message2)
		for i, dLen := range dataLen {
			messageDrone=messageDroneProto[:dLen]
			messageDrone[0]= byte(i)
			time.Sleep(temp[i])
			t.PrintDroneClient(time.Now().UnixNano(), int8(i), messageDrone) // print the timestamp
			stream[0].Write(messageDrone)
		}
		//stream[0].Write([]byte{'X'})
		t.file.Close()
	}()

	// Create the BULK message to send (GB)
	maxSendBytes := (*mb) * 1024 * 1024
	messageBulk := make([]byte, maxSendBytes) // Generate a message of PACKET_SIZE full of random information
	nBulk:=numThreads-1
	wg.Add(nBulk)
	for i:=1;i<3;i++ {
		go func(i int) {
			t:=NewTraceClient(*fileNameBulk)
			defer wg.Done()
			t.PrintDroneClient(time.Now().UnixNano(), 0, nil) // print the timestamp
			stream[i].Write(messageBulk)
			t.file.Close()
		}(i)
	}
	wg.Wait()

}
