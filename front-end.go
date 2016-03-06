package main

import (
	"fmt"
	"os"
	"net"
	"encoding/json"
)

type Replica struct {
	NodeId  string
	RPCAddr string
}

// map to where the key is the replica id and the value is the replica's IP
var replicaMap map[string]string

// Main server loop.
func main() {
	// Parse args.
	usage := fmt.Sprintf("Usage: %s [client ip:port] [replica-node ip:port]\n",
		os.Args[0])
	if len(os.Args) != 3 {
		fmt.Printf(usage)
		os.Exit(1)
	}

	replicaMap = make(map[string]string)

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

func ReplicaActivityListener() {
	
}

// Function to listen for newly connected replica nodes
func ReplicaListener(conn *net.UDPConn) {
	
	buf := make([]byte, 1024)
	for {
		readLength, _, err := conn.ReadFromUDP(buf)
		checkError(err)
		// receive Replica struct from replica node
		var replica Replica
		err = json.Unmarshal(buf[:readLength], &replica)
		if err == nil {
			fmt.Println("Replica joined: " + replica.NodeId + " @ " + replica.RPCAddr)
			replicaMap[replica.NodeId] = replica.RPCAddr
		}
	}
}

// If error is non-nil, print it out and halt.
func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error ", err.Error())
		os.Exit(1)
	}
}
