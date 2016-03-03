package main

import (
	"fmt"
	"os"
	"net"
)

// Main server loop.
func main() {
	// Parse args.
	usage := fmt.Sprintf("Usage: %s [client ip:port] [replica-node ip:port]\n",
		os.Args[0])
	if len(os.Args) != 3 {
		fmt.Printf(usage)
		os.Exit(1)
	}
	
	clientAddrString := os.Args[1]
	
	replicaAddrString := os.Args[2]
	replicaAddr, err := net.ResolveUDPAddr("udp", replicaAddrString)
	checkError(err)
	
	// start UDP server to listen for replica node activity
	replicaConn, err := net.ListenUDP("udp", replicaAddr)
	if err != nil {
		fmt.Println("Error on UDP listen: ", err)
		os.Exit(-1)
	}
	
	go ReplicaListener(replicaConn)
	
	// start TCP server to listen for new clients
	clientConn, err := net.Listen("tcp", clientAddrString)
	if err != nil {
		fmt.Println("Error on TCP listen: ", err)
		os.Exit(-1)
	}
	
	for {
		
		clientAddr, err := clientConn.Accept()
		checkError(err)
		fmt.Println(clientAddr)
		
	}
	
}

func ReplicaListener(conn *net.UDPConn) {
	
	for {
		
	}
}

// If error is non-nil, print it out and halt.
func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error ", err.Error())
		os.Exit(1)
	}
}
