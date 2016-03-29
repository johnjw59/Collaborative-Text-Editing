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
	"math/rand"
	"time"
	//"github.com/arcaneiceman/GoVector/govec"
)

type WCharacter struct {
	SiteID    string // site id and clock make up the WCharacter's unique ID
	Clock     int
	IsVisible bool
	CharVal   string // should have length 1
	PrevChar  *WCharacter
	NextChar  *WCharacter
}

// args in WriteToDoc(args)
type WriteArgs struct {
	newString *WCharacter // new string for document contents
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

// struct to represent operations
type Operation struct {
	OpChar *WCharacter
	OpType string
}
// TODO: Create pool of operations

// struct to represent a document
type Document struct {
	DocName string
	Contents *WCharacter
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

	wChar := new(WCharacter)
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
func isExecutable(op *Operation) bool {
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

func IntegrateDel(wChar *WCharacter) {
	wChar.IsVisible = false
}

func IntegrateIns(wChar *WCharacter, cPrev *WCharacter, cNext *WCharacter) {
	// TODO
}

// return the ith visible character in a string of WCharacters - what happens when i is larger than string length?
func getIthVisible(doc *Document, i int) *WCharacter {
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

func Contains(doc *Document, wChar *WCharacter) bool { // TODO
	docChar := doc.Contents

	for docChar != nil {
		if docChar.SiteID == wChar.SiteID && docChar.Clock == wChar.Clock {
			return true
		} else {
			docChar = docChar.NextChar
		}
	}

	return false
}

type ReplicaService struct {}

// Write to Doc
func (rs *ReplicaService) WriteToDoc(args *WriteArgs, reply *ValReply) error {
	// Acquire mutex for exclusive access to kvmap.
	//documentContents.Lock()
	// Defer mutex unlock to (any) function exit.
	//defer documentContents.Unlock()

	document.Contents = args.newString
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

	reply.Val = document.DocName // execute the get
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

// Stores a document based on a document id. Used for the persistent document storage
// ** Note that this function currently just initializes the key in the storage replica's
// document map and does not save any actual document data
func (rs *ReplicaService) StoreDocument(args *StorageArgs, reply *ValReply) error {
	documentId := args.DocumentId
	documentsMap[documentId] = "some test value"
	fmt.Println("Stored document: " + documentId)
	reply.Val = "success"
	return nil
}

// Retrieves a document based on a document id. Used for the persistent document storage
// TODO: return a Document object
func (rs *ReplicaService) RetrieveDocument(args *StorageArgs, reply *ValReply) error {
	documentId := args.DocumentId
	document, ok := documentsMap[documentId]
	
	if ok {
		fmt.Println("Retrieved document: " + documentId)
		reply.Val = document
	} else {
		fmt.Println("Document " + documentId + " does not exist")
	}
	
	return nil
}

var activeReplicasMap map[string]string
var documentsMap map[string]string // TODO: change value type to Document
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
	replicaID = os.Args[3]
	replicaClock = 0

	replicaAddr, err := net.ResolveUDPAddr("udp", replicaAddrString)
	checkError(err)

	frontEndAddr, err := net.ResolveUDPAddr("udp", frontEndAddrString)
	checkError(err)
	
	// Connect to the front-end node
	fmt.Println("dialing to front end")
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

	// check if this replica is to be used for persistent storage
	if replicaID == "storage" {
		documentsMap = make(map[string]string)
	} else {
		// Start HTTP server
		flag.Parse()
		http.Handle("/", http.StripPrefix("/", http.FileServer(http.Dir("./"))))
		http.HandleFunc("/doc/", ServeHome)
		http.HandleFunc("/ws", ServeWS)
		go func() {
			log.Fatal(http.ListenAndServe(*httpAddress, nil))
		}()	
	}

	// testing
	runTests()

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
	} else if (strings.Split(r.URL.Path, "/doc/")[1] == "") {
		http.Error(w, "Document Id is invalid", 404)
		return
	}
	
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	homeTempl.Execute(w, "ws://" + r.Host + "/ws")
}

// Handles websocket requests from the web-app
func ServeWS(w http.ResponseWriter, r *http.Request) {
	var cmd AppMessage
	ws, err := upgrader.Upgrade(w, r, nil)
	checkError(err)

	for {
		cmd = AppMessage{}
		err := ws.ReadJSON(&cmd)
		if err != nil {
			log.Println("readWS:", err)
			break
		}
		fmt.Println(cmd)

		// Interpret message and handle different cases (ins/del)
		switch cmd.Op {
			case "init":
				documentId := CreateDocumentId(9)
				StoreDocument(documentId)
				ws.WriteMessage(websocket.TextMessage, []byte(documentId))
			case "retrieve":
				document := RetrieveDocument(cmd.Val) 
				ws.WriteMessage(websocket.TextMessage, []byte(document))
			case "ins":
				GenerateIns(cmd.Pos, cmd.Val)
			case "del":
				GenerateDel(cmd.Pos)
		}
	}

	ws.Close()
}

// Store documents based on document Id by contacting the storage replica
func StoreDocument(documentId string) {
	
	storageIP, ok := activeReplicasMap["storage"]
	if !ok {
		fmt.Println("Storage replica has not been initialized")
		return 
	}

	r, err := rpc.Dial("tcp", storageIP)
	if err != nil {
		log.Fatalf("Cannot reach Storage Replica %s\n%s", "storage", err)
		return
	}

	args := &StorageArgs{documentId}
	var result ValReply

	err = r.Call("ReplicaService.StoreDocument", args, &result)
	if err != nil {
		log.Fatalf("Error retrieving document", "storage", err)
	}	
}

// Retrieves documents based on document Id by contacting the storage replica
func RetrieveDocument(documentId string) string {
	
	storageIP, ok := activeReplicasMap["storage"]
	if !ok {
		fmt.Println("Storage replica has not been initialized")
		return ""
	}

	r, err := rpc.Dial("tcp", storageIP)
	if err != nil {
		log.Fatalf("Cannot reach Storage Replica %s\n%s", "storage", err)
	}

	args := &StorageArgs{documentId}
	var result ValReply

	err = r.Call("ReplicaService.RetrieveDocument", args, &result)
	if err != nil {
		log.Fatalf("Error retrieving document", "storage", err)
	}
	
	return result.Val
}

// construct string from a document
func constructString(doc *Document) string {
	wchar := doc.Contents
	contents_str := ""

	for wchar != nil {
		if wchar.IsVisible {
			contents_str += wchar.CharVal
		}
		wchar = wchar.NextChar
	}
	return contents_str

}

// Returns a random string of size strlen
func CreateDocumentId(strlen int) string {
	rand.Seed(time.Now().UTC().UnixNano())
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, strlen)
	for i := 0; i < strlen; i++ {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return string(result)
}

// If error is non-nil, print it out and halt.
func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error ", err.Error())
		os.Exit(1)
	}
}



// Tests

func runTests() {
	fmt.Println("Starting testing")
	constructStringTests()
	getIthVisibleTests()
}

func constructStringTests() {
	var testDoc = new(Document)
	testDoc.DocName = "testDoc"
	var docString string

	// test empty contents
	testDoc.Contents = nil
	docString = constructString(testDoc)
	fmt.Printf("Empty doc string: %s\n", docString)

	// single character
	char1 := WCharacter{
		SiteID: replicaID, 
		Clock: replicaClock, 
		IsVisible: true, 
		CharVal: "a",
		PrevChar: nil,
		NextChar: nil }

	testDoc.Contents = &char1
	docString = constructString(testDoc)
	fmt.Printf("Single char in doc: %s\n", docString)

	// add a couple more characters, including one invisible
	replicaClock += 1

	char2 := WCharacter{
		SiteID: replicaID, 
		Clock: replicaClock, 
		IsVisible: false, 
		CharVal: "b",
		PrevChar: &char1,
		NextChar: nil }

	replicaClock += 1

	char3 := WCharacter{
		SiteID: replicaID, 
		Clock: replicaClock, 
		IsVisible: true, 
		CharVal: "c",
		PrevChar: &char2,
		NextChar: nil }

	char1.NextChar = &char2
	char2.NextChar = &char3

	docString = constructString(testDoc)
	fmt.Printf("Multiple characters in doc: %s\n", docString)

	// set all to invisible
	char1.IsVisible = false
	char3.IsVisible = false
	docString = constructString(testDoc)
	fmt.Printf("All invisible chars in doc: %s\n", docString)
}

func getIthVisibleTests() {
	var testDoc = new(Document)
	testDoc.DocName = "testDoc"
	var ithVisible *WCharacter

	// test empty contents
	testDoc.Contents = nil
	ithVisible = getIthVisible(testDoc, 0)
	if ithVisible == nil {
		fmt.Println("Empty doc contents -> no 1st visible")
	}

	// single character
	char1 := WCharacter{
		SiteID: replicaID, 
		Clock: replicaClock, 
		IsVisible: true, 
		CharVal: "a",
		PrevChar: nil,
		NextChar: nil }

	testDoc.Contents = &char1
	ithVisible = getIthVisible(testDoc, 0)
	if ithVisible != nil {
		fmt.Printf("1st visible in doc with 1 char: %s\n", ithVisible.CharVal)
	}

	// add a couple more characters, including one invisible
	replicaClock += 1

	char2 := WCharacter{
		SiteID: replicaID, 
		Clock: replicaClock, 
		IsVisible: false, 
		CharVal: "b",
		PrevChar: &char1,
		NextChar: nil }

	replicaClock += 1

	char3 := WCharacter{
		SiteID: replicaID, 
		Clock: replicaClock, 
		IsVisible: true, 
		CharVal: "c",
		PrevChar: &char2,
		NextChar: nil }

	char1.NextChar = &char2
	char2.NextChar = &char3

	ithVisible = getIthVisible(testDoc, 0)
	if ithVisible != nil {
		fmt.Printf("1st visible in doc with 2 vis chars and 1 invis: %s\n", ithVisible.CharVal)
	}
	ithVisible = getIthVisible(testDoc, 1)
	if ithVisible != nil {
		fmt.Printf("2nd visible in doc with 2 vis chars and 1 invis: %s\n", ithVisible.CharVal)
	}
	ithVisible = getIthVisible(testDoc, 2)
	if ithVisible == nil {
		fmt.Printf("3rd visible in doc with 2 vis chars and 1 invis does not exist\n")
	}
}
