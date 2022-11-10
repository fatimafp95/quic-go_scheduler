package main

import (
	"crypto/tls"
	"encoding/csv"
	"flag"
	"fmt"
	"github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/internal/utils"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var t trace

type trace struct {
	fileName  string
	file      *os.File
	timeStart time.Time
}

func (t *trace) Print(tx_time time.Time, fileName string) {
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		t.file, _ = os.OpenFile(fileName, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
		fmt.Fprintf(t.file, " TX TIME \n")
	} else {
		t.file, _ = os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	}

	fmt.Fprintf(t.file, "%d\n", tx_time)
	t.file.Close()
}

func streamOpener(session quic.Connection) quic.Stream {
	stream, err := session.OpenStream()
	if err != nil {
		panic("Error opening stream")
	}
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
	fileName := flag.String("file", "", "Files name")
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
	fmt.Println("Successfully Opened file")
	defer csvFile.Close()
	data, err := csv.NewReader(csvFile).ReadAll()
	if data == nil {
		fmt.Println("Error reading the file")
	}

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
	fmt.Println("Lista de streams:", stream[0].StreamID(), stream[1].StreamID())
	// Create the message to send->Bulk message
	maxSendBytes := (*mb) * 1024 * 1024
	messageBulk := make([]byte, maxSendBytes) // Generate a message of PACKET_SIZE full of random information
	if n, err := rand.Read(messageBulk); err != nil {
		panic(fmt.Sprintf("Failed to create test message: wrote %d/%d Bytes; %v\n", n, maxSendBytes, err))
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		stream[1].Write(messageBulk)
	}()
	/*message2 := make([]byte,maxSendBytes)
	b := []byte("HOLA")
	message2 = append(message2,b...)*/
	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Println("Drone file is reading")
		/**************DRONE MESSAGES******************/
		messageDrone := make([]byte, 242)
		for i, dLen := range dataLen {
			time.Sleep(temp[i])
			t.Print(time.Now(), *fileName) // print the timestamp
			stream[0].Write(messageDrone[:dLen])
		}
	}()
	wg.Wait()
}
