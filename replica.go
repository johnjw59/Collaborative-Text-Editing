package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"text/template"
	"strings"
	//"github.com/arcaneiceman/GoVector/govec"
)

type WCharacter struct {
	siteID    string // site id and clock make up the W-Character's unique ID
	clock     int
	isVisible bool
	charVal   string // should have length 1
	prevChar  *WCharacter
	nextChar  *WCharacter
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

type StorageArgs struct {
	DocumentId string
}

// Info about the replica
type Replica struct {
	NodeId  string
	RPCAddr string
}

// Communication from web-app
type AppMessage struct {
	Op  string
	Pos int
	Val string
}

//
var ws *websocket.Conn

// string to containt contents of document -> will be changed to a different data structure later on
var documentContents string

//var documentContents []W-Character

// each replica has a logical clock associated with it
var replicaClock int

// WOOT Methods - three stage process of making changes
func GenerateIns(pos int, char string) {
	// need to increment clock
	replicaClock += 1

}

func GenerateDel(pos int) {

}

// check preconditions of operation
func isExecutable() {

}

func receiveOperation() {

}

func IntegrateDel() {

}

func IntegrateIns() {

}

// return the ith visible character in a string of W-Characters - what happens when i is larger than string length?
func getIthVisible(str []WCharacter, i int) WCharacter {
	index := 0
	count := 0

	for j, wchar := range str {
		if count == i {
			break
		}
		if wchar.isVisible {
			count += 1
		}
		index = j
	}
	return str[index]
}

type ReplicaService struct{}

// Write to Doc
func (rs *ReplicaService) WriteToDoc(args *WriteArgs, reply *ValReply) error {
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
func (rs *ReplicaService) ReadFromDoc(args *ReadArgs, reply *ValReply) error {
	// Acquire mutex for exclusive access to kvmap.
	//documentContents.Lock()
	// Defer mutex unlock to (any) function exit.
	//defer documentContents.Unlock()

	reply.Val = documentContents // execute the get
	fmt.Println("Performing Read")
	return nil
}

// Set local map of active nodes
func (rs *ReplicaService) SetActiveNodes(args *ActiveReplicas, reply *ValReply) error {
	activeReplicasMap = args.Replicas
	reply.Val = "success"
	fmt.Println("Updated map of replicas")
	return nil
}

// Retrieves a document based on a document id. Used for the persistent document storage
func (rs *ReplicaService) RetrieveDocument(args *StorageArgs, reply *ValReply) error {
	documentId := args.DocumentId
	document, ok := documentsMap[documentId]
	
	if ok {
		reply.Val = document
	} 
	return nil
}

var activeReplicasMap map[string]string
var documentsMap map[string]string
var upgrader = websocket.Upgrader{} // use default options
var httpAddress = flag.String("addr", ":8080", "http service address")
var homeTempl = template.Must(template.ParseFiles("index.html"))

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
	// Logger := govec.Initialize("client", "clientlogfile")

	activeReplicasMap = make(map[string]string)

	replicaAddrString := os.Args[1]
	frontEndAddrString := os.Args[2]
	repID := os.Args[3]

	replicaAddr, err := net.ResolveUDPAddr("udp", replicaAddrString)
	checkError(err)

	frontEndAddr, err := net.ResolveUDPAddr("udp", frontEndAddrString)
	checkError(err)

	// check if this replica is to be used for persistent storage
	if repID == "storage" {
		documentsMap = make(map[string]string)
		storageIP := strings.Split(replicaAddrString, ":")[0]
		httpAddress = flag.String("storage_addr",  storageIP + ":8080", "http storage service address")	
	}
	
	// Connect to the front-end node
	fmt.Println("dialing to front end")
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

	// Start HTTP server
	flag.Parse()
	http.Handle("/", http.StripPrefix("/", http.FileServer(http.Dir("./"))))
	http.HandleFunc("/doc/", ServeHome)
	http.HandleFunc("/ws", ServeWS)
	go func() {
		log.Fatal(http.ListenAndServe(*httpAddress, nil))
	}()

	// handle RPC calls from other Replicas
	rpc.Register(&ReplicaService{})
	r, err := net.Listen("tcp", replicaAddrString)
	checkError(err)
	for {
		conn, err := r.Accept()
		checkError(err)
		go rpc.ServeConn(conn)
	}
}

// Serve the home page at localhost:8080
func ServeHome(w http.ResponseWriter, r *http.Request) {
	
	if !strings.HasPrefix(r.URL.Path, "/doc/") {
		http.Error(w, "Not found", 404)
		return
	} else {
		RetrieveDocument(strings.Split(r.URL.Path, "/doc/")[1])
	}
	
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	homeTempl.Execute(w, "ws://"+r.Host+"/ws")
}

// Handles websocket requests from the web-app
func ServeWS(w http.ResponseWriter, r *http.Request) {
	var cmd AppMessage
	ws, err := upgrader.Upgrade(w, r, nil)
	checkError(err)

	for {
		err := ws.ReadJSON(&cmd)
		if err != nil {
			log.Println("readWS:", err)
			break
		}

		// Interpret message and handle different cases (ins/del)
		fmt.Println(cmd)
	}

	ws.Close()
}

// Retrieves documents based on document Id by contacting the storage replica
func RetrieveDocument(documentId string) {
	r, err := rpc.Dial("tcp", activeReplicasMap["storage"])
	if err != nil {
		log.Fatalf("Cannot reach Storage Replica %s\n%s", "storage", err)
	}

	args := &StorageArgs{documentId}
	var result ValReply

	err = r.Call("ReplicaService.RetrieveDocument", args, &result)
	if err != nil {
		log.Fatalf("Error retrieving document", "storage", err)
	}
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
