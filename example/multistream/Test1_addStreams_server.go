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
	"time"

	//	"net"
	"sync"
	//	"time"
)

func streamCreator2 (sess quic.Connection, wg sync.WaitGroup) {

	defer wg.Done()

	var end time.Time
	var bytesReceived int
	buf := make([]byte, 2621440) // Max. amount of data per stream...

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

			if err := conn.SetReadDeadline(end.Add(20 * time.Second)); err != nil {
				fmt.Println("Could not set connection read deadline: " + err.Error())
			}

			if n, err := conn.Read(buf); err != nil {
				break
			} else {
				bytesReceived += n
			}
			fmt.Println("Number of bytes received: ",bytesReceived)
		}
		fmt.Println("Read deadline reached, finishing")
	}

	stream, err := sess.AcceptStream(context.Background())
	if err != nil {
		fmt.Println("Connection closed with error "+err.Error())
	}else{
		fmt.Println("Stream accepted: ",stream.StreamID())
	}
	//To know how many bytes the server is receiving from the client (???)
	receiveAsMuchAsPossible(stream)

	//It should show the total amount of bytes :')
	var numBytes int64
	numBytes, err = io.Copy(stream,stream)
	fmt.Println("Stream ID:",stream.StreamID(),"\nTotal amount of bytes per each stream: ", numBytes)

	// Close stream
	if err = stream.Close(); err != nil {
		fmt.Println("Failed to close the stream")
	}
}

func main() {

	// Listen on the given network address for QUIC connection
	ip := flag.String("ip", "localhost:4242", "IP:Port Address")
	numStreams:= flag.Int("numStreams",1, "Number of streams to use")
	flag.Parse()

	//QUIC config
	quicConfig := &quic.Config{
		AcceptToken: AcceptToken2,
	}
	sess_chann := make(chan quic.Connection)

	//Goroutines things
	var wg sync.WaitGroup


	listener, err := quic.ListenAddr(*ip, GenerateTLSConfig(), quicConfig)
	fmt.Println("Server listening...")
	if err != nil {
		panic(err)
	}

	//Connection is accepted by the server...
	go func() {
		for {
		sess, err := listener.Accept(context.Background())
		fmt.Println("Connection accepted")

		if err != nil {
			fmt.Println("Error accepting: ", err.Error())
			return
		}
		sess_chann <- sess
		}
	}()

	sess := <-sess_chann //QUIC SESSION
	fmt.Println("\nEstablished QUIC connection\n")

	//Accepting and reading streams...
	for i:=1;i<=*numStreams;i++ {
		wg.Add(1)
		go func() {
			streamCreator2(sess, wg)
		}()
	}
	wg.Wait()

	defer listener.Close()

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