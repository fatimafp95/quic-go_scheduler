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
	"sync"
	"time"
)

func streamCreator (sess quic.Connection) {
	stream, err := sess.AcceptStream(context.Background())
	_, err = io.Copy(loggingWriter{Writer: stream, id: stream.StreamID()}, stream)
	if err != nil {
		panic(err)
	}
	var n int64
	n, err = io.Copy(stream,stream)
	fmt.Println("Stream ID: %v, %d bytes", stream.StreamID(), n)
}
func main() {

	// Listen on the given network address for QUIC connection
	ip := flag.String("ip", "localhost:4242", "IP:Port Address")
	flag.Parse()
	quicConfig := &quic.Config{
		AcceptToken: AcceptToken,
	}
	sess_chann := make(chan quic.Connection)
	var wg sync.WaitGroup

	listener, err := quic.ListenAddr(*ip, generateTLSConfig(), quicConfig)
	fmt.Println("Server listening...")
	if err != nil {
		panic(err)
	}
	wg.Add(1)

	go func() {
		sess, err := listener.Accept(context.Background())
		fmt.Println("Connection accepted")

		if err != nil {
			fmt.Println("Error accepting: ", err.Error())
			return
		}
		sess_chann <- sess
	}()
	defer listener.Close()

	sess := <-sess_chann //QUIC SESSION
	fmt.Println("\nEstablished QUIC connection\n")

	for i:=1;i<=4;i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			streamCreator(sess)
		}()
	}
	wg.Wait()

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
// A wrapper for io.Writer that also logs the message.
type loggingWriter struct{
	io.Writer
	id quic.StreamID
}

func (w loggingWriter) Write(b []byte) (int, error) {
	fmt.Printf("Server (stream %v): Got '%s'\n", w.id, string(b))
	return w.Writer.Write(b)
}
// Setup a bare-bones TLS config for the server
func generateTLSConfig() *tls.Config {
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
