package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"github.com/lucas-clemente/quic-go"
	"io"
	"math/big"
	"net"
	"os"
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
func (t *trace) PrintDrone(tx_time int64) {
	fmt.Fprintf(t.file, "%d\n", tx_time)
}

func streamCreator(sess quic.Connection, mb int, fileNameBulk string) int {
	totalBytesReceived:=0
	auxBuf := make([]byte,1)
	outfile, err := os.OpenFile("outServer.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer outfile.Close()

	stream, err := sess.AcceptStream(context.Background())
	if err != nil {
		fmt.Fprintln(outfile,"Connection closed with error " + err.Error())
	} else {
		fmt.Fprintln(outfile,"Stream accepted: ", stream.StreamID())
	}
	stream.Read(auxBuf)
	id := stream.StreamID()
	if err!= nil{
		panic(err)
	}
	var t *trace
	var timeStamp int64
	if id != 0{
		bytesReceived:=0

		//No priority stream
		buf := make([]byte, 1024) // Buffer for each stream
		for {
			if n, err := io.ReadFull(stream,buf); err != nil {
				break
			} else {
				bytesReceived += n
			}
		}
		fmt.Fprintln(outfile,"Server - Number of bytes received by the stream ",stream.StreamID(),":", bytesReceived)
		totalBytesReceived=bytesReceived
	} else if id==0{
		t = NewTrace(fileNameBulk)
		bytesReceived:=0
		//No priority stream
		buf := make([]byte, 10485760) // Buffer for each stream
		for {
			if n, err := io.ReadFull(stream,buf); err != nil {
				fmt.Fprintln(outfile,"Error stream 0: ",err)
				break
			} else {
				bytesReceived += n
			}
			if bytesReceived >= 10485760 {
				timeStamp = time.Now().UnixNano()
				t.PrintDrone(timeStamp)
				fmt.Fprintln(outfile,"TS of BULK at the server side\n")
				break
			}
		}
		fmt.Fprintln(outfile,"Server - Number of bytes received by the stream ",stream.StreamID(),":", bytesReceived)
		totalBytesReceived=bytesReceived

		t.file.Close()
	}
	//To know how many bytes the server is receiving from the streams
//
	return totalBytesReceived

}
func main() {

	// Listen on the given network address for QUIC connection
	ip := flag.String("ip", "localhost:4242", "IP:Port Address")
	numStreams := flag.Int("ns", 1, "Number of streams to use")
	mb := flag.Int("mb", 1, "File size in MiB")
	fileNameBulk := flag.String("fileBulk", "", "Files name")
	flag.Parse()

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
//	fmt.Println("Server: Connection accepted")
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
			n:= streamCreator(sess, *mb, *fileNameBulk)
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
