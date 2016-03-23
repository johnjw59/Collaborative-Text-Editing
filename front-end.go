package main

import (
	"fmt"
	"os"
	"net"
	"encoding/json"
	"net/http"
	"log"
	"bytes"
	gorillaJson "github.com/gorilla/rpc/json"
)

// Reply from service for all API calls
type ValReply struct {
	Val string // value; depends on the call
}

// Info about other active replicas
type ActiveReplicas struct {
	Replicas map[string]string
}

// Info about the replica
type Replica struct {
	NodeId  string
	RPCAddr string
}

// key is the replica id and the value is the replica's RCP IP
var replicaRPCMap map[string]string

// Main server loop.
func main() {
	// Parse args.
	usage := fmt.Sprintf("Usage: %s [front-end ip:port]\n",
		os.Args[0])
	if len(os.Args) != 2 {
		fmt.Printf(usage)
		os.Exit(1)
	}
	
	replicaRPCMap = make(map[string]string)

	replicaAddrString := os.Args[1]
	replicaAddr, err := net.ResolveUDPAddr("udp", replicaAddrString)
	checkError(err)
	
	// start UDP server to listen for replica node activity
	replicaConn, err := net.ListenUDP("udp", replicaAddr)
	if err != nil {
		fmt.Println("Error on UDP listen: ", err)
		os.Exit(-1)
	}
	ReplicaListener(replicaConn)
}

// Listen for newly connected replica nodes
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
			replicaRPCMap[replica.NodeId] = replica.RPCAddr
			UpdateReplicas()
		} else {
			checkError(err)
		}
	}
}

// Sends the map of active replicas to all replicas.
// Encodes an HTTP request as an RPC request as gorilla/rpc doesn't give us proper client implementation
func UpdateReplicas() {
	for _, RPCAddress := range replicaRPCMap {
		
		url := "http://" + RPCAddress + "/rpc"
		args := &ActiveReplicas{replicaRPCMap}
		
		message, err := gorillaJson.EncodeClientRequest("ReplicaService.SetActiveNodes", args)
		if err != nil {
			log.Fatalf("%s", err)
		}
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(message))
		if err != nil {
			log.Fatalf("%s", err)
		}
		req.Header.Set("Content-Type", "application/json")
		client := new(http.Client)
		resp, err := client.Do(req)
		if err != nil {
			log.Fatalf("Error in sending request to %s. %s", url, err)
		}
		defer resp.Body.Close()

		var result ValReply
		err = gorillaJson.DecodeClientResponse(resp.Body, &result)
		if err != nil {
			log.Fatalf("Couldn't decode response. %s", err)
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
