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
	"math/big"
	"net"
	"os"
//	"strconv"
//	"strings"
	"sync"
	"time"
)
//SERVER
var t trace
type trace struct{
	fileName string
	file *os.File
	timeStart time.Time
}
func (t *trace) PrintServer(tx_time int64, fileName string) {
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		t.file, _ = os.OpenFile(fileName, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
		fmt.Fprintf(t.file, " TX TIME \n")
	} else {
		t.file, _ = os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	}

	fmt.Fprintf(t.file,"%d\n",tx_time)
	t.file.Close()
}
func (t *trace) PrintBulk(tx_time int64, fileName string) {
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		t.file, _ = os.OpenFile(fileName, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
		fmt.Fprintf(t.file, " TX TIME \n")
	} else {
		t.file, _ = os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	}

	fmt.Fprintf(t.file,"Bulk: %d\n",tx_time)
	t.file.Close()
}
func streamCreator(sess quic.Connection, fileName string) int {

	var bytesReceived int
	buf := make([]byte, 10486784) // Max. amount of data per stream...

	// As Pablo did...
	type readFromConn interface {
		// SetReadDeadline sets the deadline for future Read calls and
		// any currently-blocked Read call.
		// A zero value for t means Read will not time out.
		SetReadDeadline(t time.Time) error

		//io.Reader
		Read(p []byte) (n int, err error)
	}
	receiveAsMuchAsPossible := func(conn readFromConn, streamID quic.StreamID) {
		for {
			if conn == nil {
				fmt.Println("Connection not found, surely closed.")
				break
			}
			if err := conn.SetReadDeadline(time.Now().Add(3*time.Millisecond)); err != nil {
				fmt.Println("Could not set connection read deadline: " + err.Error())
			}

			if n, err := conn.Read(buf); err != nil {
				break
			} else {
				bytesReceived += n
				fmt.Println("StreamID:", streamID)
				fmt.Println(n)
				fmt.Println(buf[n-4])
				/*if buf[n-2] == 0 && buf[n-1]==1 {
					t.PrintServer(time.Now().UnixNano(), fileName)
				}else if buf[n-4] == 'H'  {
					fmt.Println("HOLA")
					t.PrintServer(time.Now().UnixNano(), fileName)
				}else{*/
					t.PrintBulk(time.Now().UnixNano(), fileName)
				//}
			}

			fmt.Println("Server - Number of bytes received: ",bytesReceived)
		}

		fmt.Println("Read deadline reached, finishing")
	}



	stream, err := sess.AcceptStream(context.Background())
	if err != nil {
		fmt.Println("Connection closed with error "+err.Error())
	}else{
		fmt.Println("Stream accepted: ",stream.StreamID())
	}

	//To know how many bytes the server is receiving from the client
	receiveAsMuchAsPossible(stream, stream.StreamID())

	return bytesReceived
}

func main(){

	// Listen on the given network address for QUIC connection
	ip := flag.String("ip", "localhost:4242", "IP:Port Address")
	numStreams:= flag.Int("ns",1, "Number of streams to use")
	mb := flag.Int("mb", 1, "File size in MiB")
	fileName := flag.String("file","","Files name")
//	scheduler := flag.String("scheduler", "rr", "Scheduler type: rr=Round Robin, wfq=Weighted Fair queueing, abs=Absolute Priorization")
//	order := flag.String("order", "1", "Weight or position to process each stream.")
	flag.Parse()

	/*//Creation of the priorizaton slice
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
	fmt.Print(slice)
	fmt.Println("Server:quic config...")
*/
	//QUIC config
	quicConfig := &quic.Config{
		AcceptToken: AcceptToken,
		//TypePrior: *scheduler,
		//StreamPrior: slice,
	}


	//Goroutines things
	var wg sync.WaitGroup
	fmt.Println("Server ready...")

	listener, err := quic.ListenAddr(*ip, GenerateTLSConfig(), quicConfig)
	defer listener.Close()

	fmt.Println("Server listening...")
	if err != nil {
		panic(err)
	}

	sess, err := listener.Accept(context.Background())
	fmt.Println("Server: Connection accepted")
	if err != nil {
		fmt.Println("Server: Error accepting: ", err.Error())
		return
	}
	fmt.Println("\nEstablished QUIC connection\n")

	//Accepting and reading streams...
	numThreads := *numStreams
	maxBytes:=0
	wg.Add(numThreads)
	for i:=1;i<=*numStreams;i++ {
		go func() {
			defer wg.Done()
			maxBytes+=streamCreator(sess, *fileName)
		}()
	}
	wg.Wait()
	if maxBytes==(*mb)*1024*1024{
		fmt.Println("Maximum of bytes received:", maxBytes)
		endTX:=time.Now().UnixNano()
		fmt.Println(endTX)
		t.PrintServer(endTX,*fileName)
	}
	fmt.Println("MaxBytes until now:",maxBytes)
}

type Token struct {
	// IsRetryToken encodes how the client received the token. There are two ways:
	// * In a Retry packet sent when trying to establish a new connection.
	// * In a NEW_TOKEN frame on a previous connection.
	IsRetryToken bool
	RemoteAddr   string
	SentTime     time.Time
}

func AcceptToken (clientAddr net.Addr,  Token *quic.Token ) bool{
	if clientAddr == nil{
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