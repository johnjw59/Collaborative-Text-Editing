package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"time"
)

// Reply from service for all API calls
type ValReply struct {
	Val string // value; depends on the call
}

// Info about other active replicas
type ActiveReplicas struct {
	Replicas map[int]Replica
}

// Info about the replica
type Replica struct {
	NodeId  int
	RPCAddr string
	DocumentId string
}

// key is the replica id and the value is the ReplicaType
var replicaRPCMap map[int]Replica

// Main server loop.
func main() {
	// Parse args.
	usage := fmt.Sprintf("Usage: %s [front-end ip:port]\n",
		os.Args[0])
	if len(os.Args) != 2 {
		fmt.Printf(usage)
		os.Exit(1)
	}

	replicaRPCMap = make(map[int]Replica)

	replicaAddrString := os.Args[1]
	replicaAddr, err := net.ResolveUDPAddr("udp", replicaAddrString)
	checkError(err)

	// start UDP server to listen for replica node activity
	replicaConn, err := net.ListenUDP("udp", replicaAddr)
	if err != nil {
		fmt.Println("Error on UDP listen: ", err)
		os.Exit(-1)
	}
	
	go ReplicaActivityListener()
	ReplicaListener(replicaConn)
}

// Listen for newly connected replica nodes
func ReplicaListener(conn *net.UDPConn) {

	buf := make([]byte, 1024)
	for {
		readLength, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			fmt.Printf("Error read from UDP %s\n", err)
		}
		// receive Replica struct from replica node
		var replica Replica
		err = json.Unmarshal(buf[:readLength], &replica)
		if err == nil {
			fmt.Printf("Replica joined: %d @ " + replica.RPCAddr + "\n", replica.NodeId)
			replicaRPCMap[replica.NodeId] = replica
			UpdateReplicas()
		} else {
			fmt.Printf("Error unmarshalling replica %s\n", err)
		}
	}
}

// Periodically check each replica for availability (every second)
// Simple, but probably not as robust as it should be
func ReplicaActivityListener() {

	for {
		for nodeId, replica := range replicaRPCMap {
			_, err := rpc.Dial("tcp", replica.RPCAddr)
			if err != nil {
				delete(replicaRPCMap, nodeId)
				fmt.Printf("Replica removed: %d from map of replicas \n", nodeId)
				UpdateReplicas()
			}
		}
		
		time.Sleep(1000 * time.Millisecond)
	}
}

// Sends the map of active replicas to all replicas.
func UpdateReplicas() {
	for id, replica := range replicaRPCMap {
		r, err := rpc.Dial("tcp", replica.RPCAddr)
		if err != nil {
			fmt.Printf("Cannot reach Replica %s\n%s\n", id, err)
			continue
		}

		args := &ActiveReplicas{replicaRPCMap}
		var result ValReply

		err = r.Call("ReplicaService.SetActiveNodes", args, &result)
		if err != nil {
			fmt.Print("Error updating Replica %s\n%s\n", id, err)
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
