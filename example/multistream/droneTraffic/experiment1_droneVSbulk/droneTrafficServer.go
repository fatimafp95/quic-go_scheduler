package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/csv"
	"encoding/pem"
	"flag"
	"fmt"
	"github.com/lucas-clemente/quic-go"
	"math/big"
	"net"
	"os"
	"strconv"

	//	"strconv"
	//	"strings"
	//"encoding/csv"
	//"strconv"
	"sync"
	"time"
)

//SERVER

type trace struct {
	fileName  string
	file      *os.File
	timeStart time.Time
}

func NewTrace (fileName string) *trace{
	t := trace{fileName: fileName}
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		t.file, _ = os.OpenFile(fileName, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
		fmt.Fprintf(t.file, " TX TIME\tN\t\n")
	} else {
		t.file, _ = os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	}
	return &t
}
func (t *trace) PrintDrone(tx_time int64, i int, i2 int) {
	fmt.Fprintf(t.file, "%d\t%d\t%d\n", tx_time, i, i2)
}

func streamCreator(sess quic.Connection, mb int, fileNameBulk string, fileNameDrone string, dataLen [163478]int) int {

	bytesReceived:=0
	auxBuf := make([]byte,1)
	stream, err := sess.AcceptStream(context.Background())
	defer stream.Close()
	if err != nil {
		fmt.Println("Connection closed with error " + err.Error())
	} else {
		fmt.Println("Stream accepted: ", stream.StreamID())
	}
	stream.Read(auxBuf)
	id := stream.StreamID()
	var t *trace
	if id != 0{
		t = NewTrace(fileNameBulk)
		buffSize := 10*1024*1024
		//No priority stream
		buf := make([]byte, buffSize) // Buffer for each stream
		for {
			if n, err := stream.Read(buf); err != nil {
				break
			} else {
				fmt.Println("StreamID:", stream.StreamID())
				fmt.Println(n)
				bytesReceived += n
				if (bytesReceived == 10485760){
					now:= time.Now()
					t.PrintDrone(now.UnixNano(),0, 0)
					fmt.Println("TS of BULK at the server side")
				}
			}
		}
		fmt.Println("Server - Number of bytes received by the stream ",stream.StreamID(),":", bytesReceived)
	}else if id==0{
		t = NewTrace(fileNameDrone)

		//Priority stream
		buf := make([]byte, 242)
		var n int
		readLoop:
		for _,dLen := range dataLen {
			lenMessage := dLen
			for n=0; n<lenMessage; {
				m, err := stream.Read(buf[n:lenMessage])
				if  err != nil {
					break readLoop
				}
				n+=m
			}
			timeStamp:=time.Now().UnixNano()

			fmt.Println("StreamID:", stream.StreamID())
			fmt.Println(n)
			bytesReceived += n

			t.PrintDrone(timeStamp, lenMessage, n)
		}
		fmt.Println("Server - Number of bytes received: ",stream.StreamID(),":", bytesReceived)
	}
	//To know how many bytes the server is receiving from the streams
	t.file.Close()
	return bytesReceived
}

func main() {

	// Listen on the given network address for QUIC connection
	ip := flag.String("ip", "localhost:4242", "IP:Port Address")
	numStreams := flag.Int("ns", 1, "Number of streams to use")
	mb := flag.Int("mb", 1, "File size in MiB")
	fileNameBulk := flag.String("fileBulk", "", "Files name")
	fileNameDrone := flag.String("fileDrone", "", "Files name")
	flag.Parse()

	//Opening drone traces file
	csvFile, err := os.Open("drone_processed.csv")
	if err != nil {
		fmt.Println("Error opening the drone.csv file")
		panic(err)
	}
//	fmt.Println("Successfully Opened file")
	defer csvFile.Close()
	data, err := csv.NewReader(csvFile).ReadAll()
	if data == nil {
		fmt.Println("Error reading the file")
	}

	var dataLen [163478]int
	for i, line := range data {
		dataLen[i], _ = strconv.Atoi(line[1])
	}

	//QUIC config
	quicConfig := &quic.Config{
		AcceptToken: AcceptToken,
	}

	//Goroutines things
	var wg sync.WaitGroup
	//fmt.Println("Server ready...")

	listener, err := quic.ListenAddr(*ip, GenerateTLSConfig(), quicConfig)
	defer listener.Close()

	fmt.Println("Server listening...")
	if err != nil {
		panic(err)
	}

	sess, err := listener.Accept(context.Background())
	defer sess.CloseWithError(0, "")
	fmt.Println("Server: Connection accepted")
	if err != nil {
		fmt.Println("Server: Error accepting: ", err.Error())
		return
	}
	fmt.Println("\nEstablished QUIC connection\n")

	//Accepting and reading streams...
	numThreads := *numStreams
	maxBytes := 0
	var mutex sync.Mutex
	wg.Add(numThreads)
	// read other streams
	for i := 0; i < numThreads; i++ {
		go func() {
			defer wg.Done()
			n:= streamCreator(sess, *mb, *fileNameBulk,*fileNameDrone,dataLen)
			mutex.Lock()
			maxBytes += n
			mutex.Unlock()
		}()
	}
	wg.Wait()
	fmt.Println("MaxBytes until now:", maxBytes)
}

type Token struct {
	// IsRetryToken encodes how the client received the token. There are two ways:
	// * In a Retry packet sent when trying to establish a new connection.
	// * In a NEW_TOKEN frame on a previous connection.
	IsRetryToken bool
	RemoteAddr   string
	SentTime     time.Time
}

func AcceptToken(clientAddr net.Addr, Token *quic.Token) bool {
	if clientAddr == nil {
		return true
	}
	if Token == nil {
		return true
	}
	return true
}

// Setup a bare-bones TLS config for the server
func GenerateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"quic-echo-example"},
	}
}
