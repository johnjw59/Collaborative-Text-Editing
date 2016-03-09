package main

import (
	"fmt"
	"os"
	"net"
	"flag"
	"log"
	"net/http"
	"encoding/json"
	"github.com/gorilla/rpc"
	gorillaJson "github.com/gorilla/rpc/json"
)


// args in WriteToDoc(args)
type WriteArgs struct {
	newString string // new string for document contents
}

// args in ReadFromDoc(args)
type ReadArgs struct {
}

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

// string to containt contents of document -> will be changed to a different data structure later on
var documentContents string

type ReplicaService struct {}

// Write to Doc
func (rs *ReplicaService) WriteToDoc(r *http.Request, args *WriteArgs, reply *ValReply) error {
	// Acquire mutex for exclusive access to kvmap.
	//documentContents.Lock()
	// Defer mutex unlock to (any) function exit.
	//defer documentContents.Unlock()

	documentContents = args.newString
	reply.Val = ""
	fmt.Println("Performing Write")
	return nil
}


// Read from Doc
func (rs *ReplicaService) ReadFromDoc(r *http.Request, args *ReadArgs, reply *ValReply) error {
	// Acquire mutex for exclusive access to kvmap.
	//documentContents.Lock()
	// Defer mutex unlock to (any) function exit.
	//defer documentContents.Unlock()

	reply.Val = documentContents // execute the get
	fmt.Println("Performing Read")
	return nil
}

// Set local map of active nodes 
func (rs *ReplicaService) SetActiveNodes(r *http.Request, args *ActiveReplicas, reply *ValReply) error {
	
	activeReplicasMap = args.Replicas	
	reply.Val = "success"
	fmt.Println("Updated map of replicas")
	return nil	
}

var activeReplicasMap map[string]string

// Main server loop.
func main() {
	// Parse args.
	usage := fmt.Sprintf("Usage: %s [replica ip:port] [front-end ip:port] [replica ID]\n",
		os.Args[0])
	if len(os.Args) != 4 {
		fmt.Printf(usage)
		os.Exit(1)
	}
	
	activeReplicasMap = make(map[string]string)
	
	replicaAddrString := os.Args[1]
	frontEndAddrString := os.Args[2]
	repID := os.Args[3]

	replicaAddr, err := net.ResolveUDPAddr("udp", replicaAddrString)
	checkError(err)

	frontEndAddr, err := net.ResolveUDPAddr("udp", frontEndAddrString)
	checkError(err)

	fmt.Println("dialing to front end")
	// Connect to the front-end node
	conn, err := net.DialUDP("udp", replicaAddr, frontEndAddr)
	checkError(err)

	replica := Replica{repID, replicaAddrString}
	jsonReplica, err := json.Marshal(replica)

	// send info about replica to front end node
	fmt.Println("Writing to udp")
	_, err = conn.Write(jsonReplica[:])
	if err != nil {
		fmt.Println("Error on write: ", err)
	}

	// Initialize contents
	documentContents = ""

	// handle RPC calls from clients
	address := flag.String("address", replicaAddrString, "")
	server := rpc.NewServer()
	server.RegisterCodec(gorillaJson.NewCodec(), "application/json")
	server.RegisterCodec(gorillaJson.NewCodec(), "application/json;charset=UTF-8")
	server.RegisterService(new(ReplicaService), "")
	http.Handle("/rpc/", server)
	log.Fatal(http.ListenAndServe(*address, nil))
}

// If error is non-nil, print it out and halt.
func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error ", err.Error())
		os.Exit(1)
	}
}
