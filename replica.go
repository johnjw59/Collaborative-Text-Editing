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
	"github.com/arcaneiceman/GoVector/govec"
)

type W-Character struct {
	SiteID string // site id and clock make up the W-Character's unique ID
	Clock int
	IsVisible boolean
	CharVal string // should have length 1
	PrevChar *W-Character
	NextChar *W-Character
}

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

// struct to represent operations
type Operation struct {
	OpChar *W-Character
	OpType string
}
// TODO: Create pool of operations

// struct to represent a document
type Document struct {
	DocName string
	Contents *W-Character
}
var document *Document // placeholder document, should have a listing of docs

// each replica has a unique id
var replicaID string

// each replica has a logical clock associated with it
var replicaClock int

// WOOT Methods - three stage process of making changes
func GenerateIns(pos int, char string) {
	// need to increment clock
	replicaClock += 1

	cPrev := getIthVisible(document, pos) // TODO: change document arg
	cNext := getIthVisible(document, pos + 1)

	wChar := new(W-Character)
	wChar.SiteID = replicaID
	wChar.Clock = replicaClock
	wChar.IsVisible = true
	wChar.CharVal = char
	wChar.PrevChar = cPrev
	wChar.NextChar = cNext

	IntegrateIns(wChar, cPrev, cNext)
	// TODO: broadcast ins(wchar)
}

func GenerateDel(pos int) {
	wChar := getIthVisible(document, pos) // TODO: change document arg

	IntegrateDel(wChar)
	// TODO: broadcast del(wchar)
}

// check preconditions of operation
func isExecutable(op *Operation) {
	wChar := op.OpChar

	if op.OpType == "del" {
		return Contains(document, wChar) // TODO: change document arg
	} else {
		return Contains(document, wChar.PrevChar) && Contains(document, wChar.NextChar) // TODO: change document arg
	}
}

func receiveOperation(op *Operation) {
	// TODO: add op to pool
}

func IntegrateDel(wChar *W-Character) {
	wChar.IsVisible = false
}

func IntegrateIns(wChar *W-Character, cPrev *W-Character, cNext *W-Character) {
	// TODO
}
 
// return the ith visible character in a string of W-Characters - what happens when i is larger than string length?
func getIthVisible(doc *Document, i int) *W-Character {
	index := 0
	wChar := doc.Contents

	for wChar != nil { // TODO: check termination conditions
		if index == i && wChar.IsVisible { // found ith visible
			return wChar
		} else if !wChar.IsVisible{ // current character is not visible, don't increment index
			wChar = wChar.NextChar
		} else { // current character is not ith but is visible
			wChar = wChar.NextChar
			index += 1
		}
	}

	return nil // no ith visible character
}

func Contains(doc *Document, wChar *W-Character) boolean { // TODO
	docChar = doc.Contents

	for docChar != nil {
		if docChar.SiteID == wChar.SiteID && docChar.Clock == wChar.Clock {
			return true
		} else {
			docChar = docChar.nextChar
		}
	}

	return false
}

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

	// govector library for vector clock logs
	Logger := govec.Initialize("client", "clientlogfile")
	
	activeReplicasMap = make(map[string]string)
	
	replicaAddrString := os.Args[1]
	frontEndAddrString := os.Args[2]
	replicaID = os.Args[3]

	replicaAddr, err := net.ResolveUDPAddr("udp", replicaAddrString)
	checkError(err)

	frontEndAddr, err := net.ResolveUDPAddr("udp", frontEndAddrString)
	checkError(err)

	fmt.Println("dialing to front end")
	// Connect to the front-end node
	conn, err := net.DialUDP("udp", replicaAddr, frontEndAddr)
	checkError(err)

	replica := Replica{replicaID, replicaAddrString}
	jsonReplica, err := json.Marshal(replica)

	// send info about replica to front end node
	fmt.Println("Writing to udp")
	_, err = conn.Write(jsonReplica[:])
	if err != nil {
		fmt.Println("Error on write: ", err)
	}

	// Initialize contents
	// documentContents = ""
	document = nil

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


// Tests
func ithVisibleTest() {

}
