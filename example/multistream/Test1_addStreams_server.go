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
	"time"

	//	"net"
	"sync"
	//	"time"
)

var t2 trace2
type trace2 struct{
	fileName string
	file *os.File
	timeStart time.Time
}
func (t2 *trace2) PrintServer(tx_time int64, fileName string) {
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		t2.file, _ = os.OpenFile(fileName, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
		fmt.Fprintf(t2.file, " TX TIME \n")
	} else {
		t2.file, _ = os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	}

	fmt.Fprintf(t2.file,"%f\n",tx_time)
	t2.file.Close()
}

func streamCreator2 (sess quic.Connection, mb int, fileName string) int{

	var end time.Time
	var bytesReceived int

	buf := make([]byte, 1048576) // Max. amount of data per stream...

	// As Pablo did...
	type readFromConn interface {
		// SetReadDeadline sets the deadline for future Read calls and
		// any currently-blocked Read call.
		// A zero value for t means Read will not time out.
		SetReadDeadline(t time.Time) error

		//io.Reader
		Read(p []byte) (n int, err error)
	}
	receiveAsMuchAsPossible := func(conn readFromConn) {
		for {
			end = time.Now()

			if conn == nil {
				fmt.Println("Connection not found, surely closed.")
				break
			}
			if err := conn.SetReadDeadline(end.Add(1 * time.Second)); err != nil {
				fmt.Println("Could not set connection read deadline: " + err.Error())
			}

			if n, err := conn.Read(buf); err != nil {
				break
			} else {
				bytesReceived += n
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
	receiveAsMuchAsPossible(stream)
	/*var maxFile int
	maxFile +=bytesReceived
	if maxFile == mb*1024*1024 {
		fmt.Println("Maximum of bytes received:", maxFile)
		endTX:=time.Now().UnixNano()
		fmt.Println(endTX)
		t2.PrintServer(endTX,fileName)
	}*/
	return bytesReceived
}

func main() {

	// Listen on the given network address for QUIC connection
	ip := flag.String("ip", "localhost:4242", "IP:Port Address")
	numStreams:= flag.Int("ns",1, "Number of streams to use")
	mb := flag.Int("mb", 1, "File size in MiB")
	fileName := flag.String("file","","Files name")

	flag.Parse()

	//QUIC config
	quicConfig := &quic.Config{
		AcceptToken: AcceptToken2,
	}

	//Goroutines things
	var wg sync.WaitGroup


	listener, err := quic.ListenAddr(*ip, GenerateTLSConfig(), quicConfig)
	defer listener.Close()

	fmt.Println("Server listening...")
	if err != nil {
		panic(err)
	}

	//Connection is accepted by the server...
	//go func() {
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
			maxBytes+=streamCreator2(sess, *mb, *fileName)
		}()
	}
	wg.Wait()
	fmt.Println("MaxBytes until now:",maxBytes)
	if maxBytes==(*mb)*1024*1024{
		fmt.Println("Maximum of bytes received:", maxBytes)
		endTX:=time.Now().UnixNano()
		fmt.Println(endTX)
		t2.PrintServer(endTX,*fileName)
	}

	//}()


	// Close stream and connection
	/*var errMsg string

	if err = sess.CloseWithError(0, ""); err != nil {
		errMsg += "; SessionErr:" + err.Error()
	}
	if errMsg != "" {
		fmt.Println("Server: Connection closed with errors" + errMsg)
	} else {
		fmt.Println("Server: Connection closed")
	}
	fmt.Println("Acabado")*/
}

type Token2 struct {
	// IsRetryToken encodes how the client received the token. There are two ways:
	// * In a Retry packet sent when trying to establish a new connection.
	// * In a NEW_TOKEN frame on a previous connection.
	IsRetryToken bool
	RemoteAddr   string
	SentTime     time.Time
}

func AcceptToken2 (clientAddr net.Addr,  Token2 *quic.Token ) bool{
	if clientAddr == nil{
		return true
	}
	if Token2 == nil {
		return true
	}
	return true
}
// A wrapper for io.Writer that also logs the message.
type loggingWriter2 struct{
	io.Writer
	id quic.StreamID
}

func (w loggingWriter2) Write(b []byte) (int, error) {
	fmt.Printf("Server (stream %v): Got '%s'\n", w.id, string(b))
	return w.Writer.Write(b)
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