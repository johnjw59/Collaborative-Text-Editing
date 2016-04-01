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
	"strconv"
	//"github.com/arcaneiceman/GoVector/govec"
)

type WCharacter struct {
	ID    []int // site id and clock make up the WCharacter's unique ID
	IsVisible bool
	CharVal   string // should have length 1
	PrevID	[]int
	NextID  []int
}

// Reply from service for all API calls
type ValReply struct {
	Val string // value; depends on the call
}

// Info about other active replicas
type ActiveReplicas struct {
	Replicas map[int]string
}

type StorageArgs struct {
	DocumentId string
}

// Info about the replica
type Replica struct {
	NodeId  int
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

// struct to represent a document
type Document struct {
	DocName string
	WString []WCharacter // ordered string of WCharacters
	WCharDic map[string]WCharacter // dictionary of WCharacters (each WCharacter must retain original next/prev)
	opPool []*Operation
}

// special start WCharacter, ID is such that it comes before every char and its nextID comes after every char
var startChar = WCharacter {
	ID: []int{0,0},
	IsVisible: true, 
	CharVal: "",
	PrevID: nil,
	NextID: []int{99999,99999} }

// special end WCharacter, ID is such that it comes after every char and its prevID comes before every char
var endChar = WCharacter {
	ID: []int{99999,99999},
	IsVisible: true, 
	CharVal: "",
	PrevID: []int{0,0},
	NextID: nil }

var document Document // placeholder document -> should eventually use the map
var replicaID int // each replica has a unique id
var replicaClock int // each replica has a logical clock associated with it
var activeReplicasMap map[int]string
var documentsMap map[string]string // TODO: change value type to Document
var upgrader = websocket.Upgrader{} // use default options
var httpAddress = flag.String("addr", ":8080", "http service address")
var homeTempl = template.Must(template.ParseFiles("index.html"))

const storageFlag int = -1

type ReplicaService struct {}

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


// Main server loop.
func main() {
	// Parse args.
	usage := fmt.Sprintf("Usage: %s [replica ip:port] [front-end ip:port] [optional storage flag, input -1 for storage] \n",
		os.Args[0])
	if len(os.Args) < 3 || len(os.Args) > 4{
		fmt.Printf(usage)
		os.Exit(1)
	}

	// govector library for vector clock logs
	// Logger := govec.Initialize("client", "clientlogfile")

	activeReplicasMap = make(map[int]string)

	// init operation pool
	//opPool = make([]*Operation, 0)
	replicaAddrString := os.Args[1]
	frontEndAddrString := os.Args[2]
	replicaID := GenerateReplicaId()
	
	if len(os.Args) == 4 {
		flag, err := strconv.Atoi(os.Args[3])
		checkError(err)
		
		if flag == storageFlag {
			replicaID = storageFlag
		} else {
			fmt.Sprintf("Usage: third argument must be -1 \n")
			os.Exit(1)
		}
	}

	// initialize clock
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
	document = Document {
		DocName: "testDoc",
		WString: []WCharacter{startChar, endChar}, // intialize to empty, only special chars exist
		WCharDic: make(map[string]WCharacter), 
		opPool: []*Operation{} }

	// add special chars to WCharDic
	document.WCharDic[strconv.Itoa(startChar.ID[0]) + "-" + strconv.Itoa(startChar.ID[1])] = startChar
	document.WCharDic[strconv.Itoa(endChar.ID[0]) + "-" + strconv.Itoa(endChar.ID[1])] = endChar


	// check if this replica is to be used for persistent storage
	if replicaID == storageFlag {
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
				// GenerateIns(cmd.Pos, cmd.Val) // TODO: Needs to be called on a specific document in map
			case "del":
				//GenerateDel(cmd.Pos)
		}
	}

	ws.Close()
}

// Store documents based on document Id by contacting the storage replica
func StoreDocument(documentId string) {
	
	storageIP, ok := activeReplicasMap[storageFlag]
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
	
	storageIP, ok := activeReplicasMap[storageFlag]
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

// Generates a random int
func GenerateReplicaId() int {
	seed := rand.NewSource(time.Now().UnixNano())
    r := rand.New(seed)
	return r.Intn(1000000)
}



// WOOT Methods - three stage process of making changes
func (doc *Document) GenerateIns(pos int, char string) {
	// need to increment clock
	replicaClock += 1

	cPrev := doc.getIthVisible(pos)
	cNext := doc.getIthVisible(pos + 1)

	if cPrev == nil || cNext == nil {
		fmt.Println("Failed to get next or prev")
		return
	}

	newWChar := WCharacter{
		ID: []int{replicaID, replicaClock},
		IsVisible: true, 
		CharVal: char,
		PrevID: cPrev.ID,
		NextID: cNext.ID }

	doc.IntegrateIns(&newWChar, cPrev, cNext)
	// TODO: broadcast ins(wchar)
}

func (doc *Document) GenerateDel(pos int) {
	wChar := doc.getIthVisible(pos)

	IntegrateDel(wChar)
	// TODO: broadcast del(wchar)
}

// check preconditions of operation
func (doc *Document) IsExecutable(op *Operation) bool {
	wChar := op.OpChar

	if op.OpType == "del" {
		return doc.Contains(wChar.ID)
	} else if op.OpType == "ins" {
		return doc.Contains(wChar.PrevID) && doc.Contains(wChar.NextID)
	} else {
		fmt.Println("Invalid operation type")
		os.Exit(1)
		return false
	}
}

func IntegrateDel(wChar *WCharacter) {
	wChar.IsVisible = false
}

func (doc *Document) IntegrateIns(cChar *WCharacter, cPrev *WCharacter, cNext *WCharacter) {
	// get all WCharacters in between cPrev and cNext
	subseqS := doc.Subsequence(*cPrev, *cNext)
	subseqString := constructString(subseqS)
	fmt.Printf("Subsequence between cPrev and cNext: %s\n", subseqString)

	// if no WCharacters in between, we're done - can insert wChar at desired position
	if len(subseqS) == 0 {
		doc.Insert(*cChar, doc.Pos(*cNext))
	} else {
		L := make([]WCharacter, 0)
		L = append(L, *cPrev)
		cPrevPos := doc.Pos(*cPrev)
		cNextPos := doc.Pos(*cNext)

		// find all characters
		for _, wChar := range subseqS {
			prev := doc.Pos(doc.WCharDic[strconv.Itoa(wChar.PrevID[0]) + "-" + strconv.Itoa(wChar.PrevID[1])])
			next := doc.Pos(doc.WCharDic[strconv.Itoa(wChar.NextID[0]) + "-" + strconv.Itoa(wChar.NextID[1])])
			if prev <= cPrevPos && next >= cNextPos {
		  		L = append(L, wChar)
			}
		  }
		L = append(L, *cNext)

		// determine order based on ID
     	i := 1
     	for i < (len(L) - 1) && CompareID(L[i].ID, cChar.ID) {
     		i += 1
     	}
     	// make recursive call using updated prev/next WChars
     	doc.IntegrateIns(cChar, &L[i-1], &L[i])
	}
}

func BroadcastOperation(op *Operation) {
	for replica := range activeReplicasMap {
		r, err := rpc.Dial("tcp", activeReplicasMap[replica])
		checkError(err)

		var result ValReply

		err = r.Call("ReplicaService.ReceiveOperation", op, &result)
		checkError(err)
		// TODO: Do something with reply?
	}
}

// receive operation from remote replica and add to local op pool
func (rs *ReplicaService) ReceiveOperation(receivedOp *Operation, reply *ValReply) error {
	fmt.Println("Receiving op: " + receivedOp.OpType)

	newOp := Operation{receivedOp.OpChar, receivedOp.OpType}
	document.opPool = append(document.opPool, &newOp)
	//document.opPool = append(document.opPool, receivedOp)

	reply.Val = ""
	return nil
}


// WOOT Helper Methods

// construct string from a document
func constructString(wString []WCharacter) string {
	contents_str := ""

	for _,wchar := range wString {
		if wchar.IsVisible {
			contents_str += wchar.CharVal
		}
	}
	return contents_str
}

// get the position of WCharacter in document's ordered WString
func (doc *Document) Pos(toFind WCharacter) int {
	for i, char := range doc.WString {
		if char.ID[0] == toFind.ID[0] && char.ID[1] == toFind.ID[1] {
			return i
		}
	}
	return -1 // toFind not in document
}

// insert WCharacter into doc's WString at position p as well as into WCharDic
func (doc *Document) Insert(char WCharacter, p int) {
	temp := WCharacter{}
	doc.WString = append(doc.WString, temp)
	copy(doc.WString[p+1:], doc.WString[p:])
	doc.WString[p] = char

	// also add to WCharDic
	doc.WCharDic[strconv.Itoa(char.ID[0]) + "-" + strconv.Itoa(char.ID[1])] = char	
}

// get subsequence of wstring between prevchar and nextchar
func (doc *Document) Subsequence(prevChar WCharacter, nextChar WCharacter) []WCharacter {
	subseq := make([]WCharacter, 0)
	startPos := doc.Pos(prevChar)
	endPos := doc.Pos(nextChar)

	for endPos >= (startPos + 2) {
		startPos += 1
		subseq = append(subseq, doc.WString[startPos])
	}
	return subseq
}

// check if document contains a wChar
func (doc *Document) Contains(wCharID []int) bool {
	for _, docChar := range doc.WString {
		if wCharID[0] == docChar.ID[0] && wCharID[1] == docChar.ID[1] {
			return true
		} 
	}
	return false
}


// return the ith visible character in a string of WCharacters
func (doc *Document) getIthVisible(i int) *WCharacter {
	index := 0

	for _, wChar := range doc.WString { // TODO: check termination conditions
		if index == i && wChar.IsVisible { // found ith visible
			return &wChar
		} else if wChar.IsVisible{ // current character is not visible, don't increment index
			index += 1
		} 
	}
	return nil // no ith visible character
}

// returns boolean representing whether ID1 < ID2
func CompareID(ID1 []int, ID2 []int) bool {
	if ID1[0] < ID2[0] {
		return true
	} else if ID1[0] == ID2[0] {
		return ID1[1] < ID2[1]
	} else {
		return false
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

func runTests() {
	fmt.Println("Starting testing")

	testDoc := Document {
		DocName: "testDoc",
		WString: []WCharacter{startChar, endChar}, // intialize to empty, only special chars exist
		WCharDic: make(map[string]WCharacter), 
		opPool: []*Operation{} }

	// add special chars to WCharDic
	testDoc.WCharDic[strconv.Itoa(startChar.ID[0]) + "-" + strconv.Itoa(startChar.ID[1])] = startChar
	testDoc.WCharDic[strconv.Itoa(endChar.ID[0]) + "-" + strconv.Itoa(endChar.ID[1])] = endChar

	posTests(&testDoc)
}

func posTests(doc *Document) {

	// single character
	replicaClock += 1
	char1 := WCharacter{
		ID: []int{replicaID, replicaClock},
		IsVisible: true, 
		CharVal: "a",
		PrevID: []int{startChar.ID[0], startChar.ID[1]},
		NextID: nil }

	replicaClock += 1
	char2 := WCharacter{
		ID: []int{replicaID, replicaClock},
		IsVisible: true, 
		CharVal: "b",
		PrevID: []int{char1.ID[0], char1.ID[1]},
		NextID: nil }

	replicaClock += 1
	char3 := WCharacter{
		ID: []int{replicaID, replicaClock},
		IsVisible: true, 
		CharVal: "c",
		PrevID: []int{startChar.ID[0], startChar.ID[1]},
		NextID: nil }

	doc.WString = []WCharacter{startChar, char1, char2, endChar}
	posChar1 := doc.Pos(char1)
	posChar2 := doc.Pos(char2)
	posChar3 := doc.Pos(char3)
	fmt.Printf("Position of char1: %d\n", posChar1)
	fmt.Printf("Position of char2: %d\n", posChar2)
	fmt.Printf("Position of char3: %d\n", posChar3)

	// print out current WString
	docString := constructString(doc.WString)
	fmt.Printf("Current WString: %s\n", docString)

	// test inserts
	fmt.Println("inserting char3 at position 2")
	doc.Insert(char3, 2)
	posChar2 = doc.Pos(char2)
	posChar3 = doc.Pos(char3)
	fmt.Printf("Position of char2: %d\n", posChar2)
	fmt.Printf("Position of char3: %d\n", posChar3)

	// print out current WString
	docString = constructString(doc.WString)
	fmt.Printf("Current WString: %s\n", docString)

	// test subsequence
	subseq := doc.Subsequence(char1, char2)
	subseqString := constructString(subseq)
	fmt.Printf("Subsequence between a and b: %s\n", subseqString)

	// another subsequence test
	replicaClock += 1
	char4 := WCharacter{
		ID: []int{replicaID, replicaClock},
		IsVisible: true, 
		CharVal: "d",
		PrevID: []int{startChar.ID[0], startChar.ID[1]},
		NextID: []int{char2.ID[0], char2.ID[0]} }

	fmt.Println("Inserting d at position 4")
	doc.Insert(char4, 4)
	docString = constructString(doc.WString)
	fmt.Printf("Current WString: %s\n", docString)

	subseq = doc.Subsequence(startChar, char4)
	subseqString = constructString(subseq)
	fmt.Printf("Subsequence between start and d: %s\n", subseqString)

	// test contains
	contains := doc.Contains(char4.ID)
	fmt.Printf("Contains d: %t\n", contains)
	contains = doc.Contains(char2.ID)
	fmt.Printf("Contains b: %t\n", contains)

	replicaClock += 1
	char5 := WCharacter{
		ID: []int{replicaID, replicaClock},
		IsVisible: false, 
		CharVal: "e",
		PrevID: []int{startChar.ID[0], startChar.ID[1]},
		NextID: []int{char2.ID[0], char2.ID[0]} }

	contains = doc.Contains(char5.ID)
	fmt.Printf("Contains e: %t\n", contains)


	// test getIthVisible
	fmt.Println("Inserting invisible e at position 2")
	doc.Insert(char5, 2)
	docString = constructString(doc.WString)
	fmt.Printf("Current WString: %s\n", docString)

	ithVisChar := doc.getIthVisible(1)
	fmt.Printf("Get 1st visible: %s\n", ithVisChar.CharVal)
	ithVisChar = doc.getIthVisible(2)
	fmt.Printf("Get 2nd visible: %s\n", ithVisChar.CharVal)
	ithVisChar = doc.getIthVisible(3)
	fmt.Printf("Get 3rd visible: %s\n", ithVisChar.CharVal)

	fmt.Printf("Setting %s to invisible\n", doc.WString[1].CharVal)
	doc.WString[1].IsVisible = false
	docString = constructString(doc.WString)
	fmt.Printf("Current WString: %s\n", docString)

	ithVisChar = doc.getIthVisible(1)
	fmt.Printf("Get 1st visible: %s\n", ithVisChar.CharVal)
	ithVisChar = doc.getIthVisible(2)
	fmt.Printf("Get 2nd visible: %s\n", ithVisChar.CharVal)


}