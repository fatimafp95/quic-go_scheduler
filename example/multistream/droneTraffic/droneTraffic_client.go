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
//Application: during 400 seconds, the drone will transmit a emulation of video traffic and, in some points, it will send traffic control. (drone traces)

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

	fmt.Fprintf(t.file,"%d\n",tx_time)
	t.file.Close()
}

func streamOpener (session quic.Connection) quic.Stream {
	stream, err := session.OpenStream()
	if (err != nil){
		panic("Error opening stream")
	}
	fmt.Println("Stream opened with the ID:",stream.StreamID())
	return stream
}

//Drone traffic: Drone packet trace captured at the 5GENESIS Athens testbed. The trace consists of packets sent between the drone and a remote controller
//over a 5G connection. The traffic was captured on-board the drone PixHawk 4.02 autopilot. The autopilot exploited UE tethering in order to connect via
//a 5G radio interface with the 5G RAN. The traffic capture is comprised of MAVLink3 packets (over UDP) sent over the 5G RAN with navigation commands.
type DroneTraces struct {
	nPkt        string//int
	tPktout     string//time.Ticker
	source      string
	sourcePort	string
	destination string
	lengthTotal string//int
	dataLen     string//int

}
func droneData(data [][]string) []DroneTraces {
	var droneTraces []DroneTraces
	for i, row := range data {
		if i > 0 {
			var record DroneTraces
			for j, field := range row {
				if j == 0 {
					record.nPkt = field
				}else if j==1{
					record.tPktout = field
				}else if j==2{
					record.source = field
				}else if j==3{
					record.destination =field
				}else if j==5 {
					record.lengthTotal = field
				}else if j==6 {
					record.dataLen = field
				}else if j==7{
					record.sourcePort = field
				}
			}
			droneTraces = append(droneTraces,record)
		}
	}

	return droneTraces
}

// NewTimer creates a new timer that is not set
func NewTimer() *utils.Timer {
	return &utils.Timer{T: time.NewTimer(time.Duration(math.MaxInt64))}
}

func main(){
	// Listen on the given network address for QUIC connection
	keyLogFile := flag.String("keylog", "", "key log file")
	ip := flag.String("ip", "localhost:4242", "IP:Port Address")
	numStreams:= flag.Int("ns",1, "Number of streams to use")
	mb := flag.Int("mb", 1, "File size in MiB")
//	fileName := flag.String("file","","Files name")
	scheduler := flag.String("scheduler", "rr", "Scheduler type: rr=Round Robin, wfq=Weight Fair queueing, abs=Absolute Priorization")
	order := flag.String("order", "1", "Weight or position to process each stream.")

	flag.Parse()
	var wg sync.WaitGroup

	//Opening drone traces file
	csvFile, err := os.Open("drone.csv")
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
	droneTrace:=droneData(data)
	if droneTrace == nil{
		fmt.Println("error")
	}

	var (
		nPkt	[167349]int
		tPktout [167349]string
		source	[167349]string
		destination [167349]string
		lengthTotal	[167349]int
		dataLen [167349]int
		sourcePort [167349]string
	)
	for i:=0; i <167349;i++{
		nPkt[i], _ =strconv.Atoi(droneTrace[i].nPkt)
		tPktout[i] = droneTrace[i].tPktout
		source[i] =droneTrace[i].source
		destination[i] = droneTrace[i].destination
		lengthTotal[i], _ =strconv.Atoi(droneTrace[i].lengthTotal)
		dataLen[i],_ = strconv.Atoi(droneTrace[i].dataLen)
		sourcePort[i] = droneTrace[i].sourcePort
	}
	var aux, aux3 string
	var aux2 []string
	var sliceTime [167349]int
	//var count1	int
	//var count2	int
	for i:=0;i<167349;i++ {
		aux = tPktout[i]
		aux2 = strings.Split(aux, ".")
		aux3 = strings.Join(aux2,"")
		sliceTime[i],_ = strconv.Atoi(aux3) //Slice de microsegundos
	}


	var sliceDifTime [167349]int64
	var diff	int64
	var positions [167349]int
	fmt.Println(len(sliceDifTime))

	for i:=0;i<len(sliceTime)-1;i++ {
		diff = int64(sliceTime[i+1] - sliceTime[i])
		if diff == 0{
			positions[i]=i
		}
		sliceDifTime[i]=diff
	}

	//Slice int con microsegundos-->duration es int64 y define nanosegundos
	var temp [167349]time.Duration
	for i, _ := range sliceDifTime {
		temp[i] = time.Duration(sliceDifTime[i] * 1000) //para hacer nano segundos
		temp[i].Milliseconds()
	}

	//Creation of the priorizaton slice
	splitString:=strings.Split(*order, "")
	lenPrior := len(*order)
	slice :=make([]int,lenPrior)
	for i:=0;i<lenPrior;i++ {
		value, err := strconv.Atoi(splitString[i])
		if err != nil {
			fmt.Println("Error creating the int slice: ", err)
		}
		slice[i]=value
	}

	//QUIC config
	quicConfig := &quic.Config{
		DisablePathMTUDiscovery: true,
		TypePrior: *scheduler,
		StreamPrior: slice,
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
	session, err := quic.DialAddr(*ip,tlsConf, quicConfig)
	if err != nil{
		panic("Failed to create QUIC session")
	}else{
		fmt.Println("QUIC session created\n")
	}


	//QUIC open streams
	numThreads := *numStreams
	var mutex sync.Mutex
	var stream [2]quic.Stream
	wg.Add(numThreads)
	for i:=0; i<numThreads;i++{
		mutex.Lock()
		go func(i int) {
			defer wg.Done()
			stream[i] = streamOpener(session)
			mutex.Unlock()
		}(i)
	}
	wg.Wait()
	fmt.Println("Lista de streams:",stream[0].StreamID(),stream[1].StreamID())
	// Create the message to send->Bulk message
	maxSendBytes :=  (*mb)*1024//*1024
	messageBulk := make([]byte, maxSendBytes) // Generate a message of PACKET_SIZE full of random information
	if n, err := rand.Read(messageBulk); err != nil {
		panic(fmt.Sprintf("Failed to create test message: wrote %d/%d Bytes; %v\n", n, maxSendBytes, err))
	}


	wg.Add(1)
	go func() {
		defer wg.Done()
		stream[1].Write(messageBulk)
	}()
	//wg.Wait()
	message2 := make([]byte,maxSendBytes)
	b := []byte("HOLA")
	message2 = append(message2,b...)
	wg.Add(1)
	go func(){
		defer wg.Done()
		fmt.Println("Drone file is reading")
		//if stream[1].StreamID() == 8{
		fmt.Println("Estoy dentro del stream 0")
		stream[0].Write(message2)

		/**************DRONE MESSAGES******************/
		//timer := NewTimer()
		/*for j:=0 ;j<len(nPkt);j++{
			if strings.HasPrefix(sourcePort[j],"58409") {
				fmt.Println("Soy la iteración:", j)
				messageDrone[dataLen[j]-1] = 1
				messageD := messageDrone[:dataLen[j]]

				fmt.Println("Longitud messageDrone: ", len(messageD))

				start := time.Now().UnixNano()
				t.Print(start, *fileName)
				stream[0].Write(messageD)
				//timeStamp:=time.Now().Add(temp[j])
				time.Sleep(temp[j])
				//fmt.Println("TimeStamp:", timeStamp)
				fmt.Println("Time Diff:", temp[j])
				/*if((time.Now().Add(time.Millisecond*5))).After(timeStamp){
					continue
				}
				timer.Reset(timeStamp) //Deadline
				<- timer.Chan()
				fmt.Print(timer.Chan())
				fmt.Println("Hola chan")
			}else{
				continue
			}*/
		//}
	}()
	wg.Wait()
}