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
	"runtime"	
	"github.com/arcaneiceman/GoVector/govec"
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
	SenderID int // ID of replying replica
	SenderClock int // (Shiviz) clock of replying replica
}

// Info about other active replicas
type ActiveReplicas struct {
	Replicas map[int]Replica
}

type StorageArgs struct {
	DocumentId string
}

type StorageReply struct {
	StoredDocument *Document
}

// Info about the replica
type Replica struct {
	NodeId  int
	RPCAddr string
	DocumentId string
}

// Communication from web-app
type AppMessage struct {
	Op  string
	Pos int
	Val string
}

// struct to represent operations
type Operation struct {
	OpChar *WCharacter
	OpType string
	Position int
	DocumentId string
	OriginatorID int
	OriginatorClock int
}

// struct to represent a document
type Document struct {
	DocName string
	WString []*WCharacter // ordered string of WCharacters
	WCharDic map[string]*WCharacter // dictionary of WCharacters (each WCharacter must retain original next/prev)
	opPool []*Operation
}

// struct to hold event information
type ShivizEvent struct {
	Event string
	HostID int
	VectorClocks map[int]int
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

var document *Document // current document being edited
var replicaID int // each replica has a unique id
var replicaClock int // each replica has a logical clock associated with it
var shivizClock int // clock to track events to be logged in ShiViz
var activeReplicasMap map[int]Replica
var documentsMap map[string]*Document // map to store different documents (used by the storage replica)
var upgrader = websocket.Upgrader{} // use default options
var httpAddress = flag.String("addr", ":8080", "http service address")
var homeTempl = template.Must(template.ParseFiles("index.html"))
var storageOpPool []*Operation
var Logger *govec.GoLog // Logger to log events for Shiviz

const storageFlag int = -1

// WebSocket protocol 
// Hub maintains the set of active connections and broadcasts messages to the connections.

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second
	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
	// Maximum message size allowed from peer.
	maxMessageSize = 1024
)

type Hub struct {
	// Registered connections.
	connections map[*Connection]string
	// Inbound messages from the connections.
	broadcast chan []byte
	// Register requests from the connections.
	register chan *Connection
	// Unregister requests from connections.
	unregister chan *Connection
}

var hub = Hub {
	broadcast:   make(chan []byte),
	register:    make(chan *Connection),
	unregister:  make(chan *Connection),
	connections: make(map[*Connection]string),
}

func (h *Hub) run() {
	for {
		select {
		case c := <-h.register:
			h.connections[c] = ""
		case c := <-h.unregister:
			if _, ok := h.connections[c]; ok {
				delete(h.connections, c)
				close(c.send)
			}
		case m := <-h.broadcast:
			for c := range h.connections {
				if h.connections[c] == document.DocName {
					select {
						case c.send <- m:
						default:
							close(c.send)
							delete(h.connections, c)
					}	
				}
			}
		}
	}
}

// Connection is an middleman between the websocket connection and the hub.
type Connection struct {
	// The websocket connection.
	ws *websocket.Conn
	// Buffered channel of outbound messages.
	send chan []byte
	// document id
	documentId string
}

// readPump pumps messages from the websocket connection to the hub.
func (c *Connection) readPump() {
	defer func() {
		hub.unregister <- c
		c.ws.Close()
	}()
	c.ws.SetReadLimit(maxMessageSize)
	c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error { c.ws.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	
	for {
		cmd := AppMessage{}
		err := c.ws.ReadJSON(&cmd)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				log.Printf("error: %v", err)
			}
			break
		}
		
		fmt.Println(cmd)
		
		// Interpret message and handle different cases (ins/del)
		switch cmd.Op {
			case "init":
				documentId := CreateDocumentId(9)
				document = InitDocument(documentId)
				hub.connections[c] = document.DocName
				message := []byte(documentId)
				hub.broadcast <- message
			case "retrieve":
				document = RetrieveDocument(cmd.Val)
				hub.connections[c] = document.DocName
				message := []byte(constructString(document.WString))
				hub.broadcast <- message
			case "ins":
				document.GenerateIns(cmd.Pos, cmd.Val)  
				message := []byte(constructString(document.WString))
				hub.broadcast <- message
			case "del":
				document.GenerateDel(cmd.Pos)
				message := []byte(constructString(document.WString))
				hub.broadcast <- message
		}
	}
}

// write writes a message with the given message type and payload.
func (c *Connection) write(mt int, payload []byte) error {
	c.ws.SetWriteDeadline(time.Now().Add(writeWait))
	return c.ws.WriteMessage(mt, payload)
}

// writePump pumps messages from the hub to the websocket connection.
func (c *Connection) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.write(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.write(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.write(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}

// Handles websocket requests from the web-app
func ServeWS(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)	
		return
	}
	
	c := &Connection {
		send: make(chan []byte, 1024),
		ws: ws }
	hub.register <- c
	go c.writePump()
	c.readPump()	
}

// RPC protocol
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
func (rs *ReplicaService) InitDocument(args *StorageArgs, reply *StorageReply) error {
	documentId := args.DocumentId
	newDocument := Document {
		DocName: documentId,
		WString: []*WCharacter{&startChar, &endChar}, // intialize to empty, only special chars exist
		WCharDic: make(map[string]*WCharacter), 
		opPool: []*Operation{} }
	
	newDocument.WCharDic[ConstructKeyFromID(startChar.ID)] = &startChar
	newDocument.WCharDic[ConstructKeyFromID(endChar.ID)] = &endChar	
	documentsMap[documentId] = &newDocument
	fmt.Println("Stored document: " + documentId)
	reply.StoredDocument = documentsMap[documentId]
	return nil
}

// function to setup RPC
func RPCService(replicaAddrString string) {
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

// Retrieves a document based on a document id. Used for the persistent document storage
// TODO: return a Document object
func (rs *ReplicaService) RetrieveDocument(args *StorageArgs, reply *StorageReply) error {
	documentId := args.DocumentId
	storedDocument, ok := documentsMap[documentId]
	
	if ok {
		fmt.Println("Retrieved document: " + documentId)
		reply.StoredDocument = storedDocument
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

	activeReplicasMap = make(map[int]Replica)

	// init operation pool
	//opPool = make([]*Operation, 0)
	replicaAddrString := os.Args[1]
	frontEndAddrString := os.Args[2]
	replicaID = GenerateReplicaId()
	
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

	// govector library for vector clock logs
	if replicaID == storageFlag {
		Logger = govec.Initialize("replica", "replica")
	} else {
		Logger = govec.Initialize("client", "client")
	}

	// initialize clocks
	replicaClock = 0
	shivizClock = 0

	replicaAddr, err := net.ResolveUDPAddr("udp", replicaAddrString)
	checkError(err)

	frontEndAddr, err := net.ResolveUDPAddr("udp", frontEndAddrString)
	checkError(err)
	
	// Connect to the front-end node
	fmt.Println("dialing to front end")
	conn, err := net.DialUDP("udp", replicaAddr, frontEndAddr)
	checkError(err)

	replica := Replica{replicaID, replicaAddrString, ""}
	jsonReplica, err := json.Marshal(replica)

	// send info about replica to front end node
	fmt.Println("Writing to udp")
	_, err = conn.Write(jsonReplica[:])
	if err != nil {
		fmt.Println("Error on write: ", err)
	}
	
	go ProcessOperations()
	
	// check if this replica is to be used for persistent storage
	if replicaID == storageFlag {
		documentsMap = make(map[string]*Document)
		storageOpPool = make([]*Operation, 0)
		RPCService(replicaAddrString)
	} else {
		go RPCService(replicaAddrString)
		
		// Start HTTP server
		flag.Parse()
		go hub.run()
		http.Handle("/", http.StripPrefix("/", http.FileServer(http.Dir("./"))))
		http.HandleFunc("/doc/", ServeHome)
		http.HandleFunc("/ws", ServeWS)
		err = http.ListenAndServe(*httpAddress, nil)
		if err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
	}
	
	// testing
	//runTests()
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

// Initialize documents with a document Id by contacting the storage replica
func InitDocument(documentId string) *Document {
	
	storageReplica, ok := activeReplicasMap[storageFlag]
	if !ok {
		fmt.Println("Storage replica has not been initialized")
		return nil
	}

	r, err := rpc.Dial("tcp", storageReplica.RPCAddr)
	if err != nil {
		log.Fatalf("Cannot reach Storage Replica %s\n%s", "storage", err)
		return nil
	}

	args := &StorageArgs{documentId}
	var result StorageReply

	err = r.Call("ReplicaService.InitDocument", args, &result)
	if err != nil {
		log.Fatalf("Error retrieving document", "storage", err)
	}

	return result.StoredDocument
}

// Retrieves documents based on document Id by contacting the storage replica
func RetrieveDocument(documentId string) *Document {
	
	storageReplica, ok := activeReplicasMap[storageFlag]
	if !ok {
		fmt.Println("Storage replica has not been initialized")
		return nil
	}

	r, err := rpc.Dial("tcp", storageReplica.RPCAddr)
	if err != nil {
		log.Fatalf("Cannot reach Storage Replica %s\n%s", "storage", err)
	}

	args := &StorageArgs{documentId}
	var result StorageReply

	err = r.Call("ReplicaService.RetrieveDocument", args, &result)
	if err != nil {
		log.Fatalf("Error retrieving document", "storage", err)
	}
	
	return result.StoredDocument
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

	cPrev, exists := doc.getIthVisible(pos)
	if !exists {
		fmt.Println("Failed to get prev")
		return
	}
	cNext, exists := doc.getIthVisible(pos + 1)
	if !exists {
		fmt.Println("Failed to get next")
		return
	}

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

	LogLocalEvent("Generating insert: " + newWChar.CharVal )

	doc.IntegrateIns(&newWChar, cPrev, cNext)

	operation := Operation{
		OpChar: &newWChar,
		OpType: "ins",
		DocumentId: doc.DocName,
		OriginatorID: replicaID,
		OriginatorClock: shivizClock}
	go BroadcastOperation(&operation)
}

func (doc *Document) GenerateDel(pos int) {
	LogLocalEvent("Generating delete at: " + strconv.Itoa(pos))

	wChar := doc.IntegrateDel(pos)

	if wChar == nil {
		fmt.Println("integrate delete failed")
		return
	}

	docString := constructString(doc.WString)
	fmt.Printf("Current WString: %s\n", docString)

	operation := Operation{
		OpType: "del",
		OpChar: wChar,
		Position: pos,
		DocumentId: doc.DocName,
		OriginatorID: replicaID,
		OriginatorClock: shivizClock}
	go BroadcastOperation(&operation)

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

// returns the character that was 'deleted'
func (doc *Document) IntegrateDel(pos int) (*WCharacter) {
	
	wChar, exists := doc.getIthVisible(pos)
	if !exists {
		fmt.Println("Failed to get ithvis")
		return nil
	}

	// don't want to delete start/end characters
	if !(isStartOrEndChar(wChar)) {
		fmt.Printf("setting %s to invisible\n", wChar.CharVal)
		wChar.IsVisible = false
	}
	
	_, exists = doc.WCharDic[ConstructKeyFromID(wChar.ID)]
	if exists {
		doc.WCharDic[ConstructKeyFromID(wChar.ID)] = wChar

		LogLocalEvent("Integrating delete: " + wChar.CharVal)
	} else {
		fmt.Printf("failed to delete -> not in map %s\n", wChar.CharVal)
	}
	
	return wChar
}

func (doc *Document) IntegrateIns(cChar *WCharacter, cPrev *WCharacter, cNext *WCharacter) {
	// get all WCharacters in between cPrev and cNext
	subseqS := doc.Subsequence(*cPrev, *cNext)

	// if no WCharacters in between, we're done - can insert wChar at desired position
	if len(subseqS) == 0 {
		fmt.Printf("itegrate insert done -> inserting at posn: %d\n ", doc.Pos(*cNext))
		doc.Insert(cChar, doc.Pos(*cNext))
		LogLocalEvent("Integrating insert: " + cChar.CharVal)
	} else {
		L := make([]*WCharacter, 0)
		L = append(L, cPrev)
		cPrevPos := doc.Pos(*cPrev)
		cNextPos := doc.Pos(*cNext)

		// find all characters
		for _, wChar := range subseqS {
			prev := doc.Pos(*doc.WCharDic[ConstructKeyFromID(wChar.PrevID)])
			next := doc.Pos(*doc.WCharDic[ConstructKeyFromID(wChar.NextID)])
			if prev <= cPrevPos && next >= cNextPos {
		  		L = append(L, wChar)
			}
		  }
		L = append(L, cNext)

		// determine order based on ID
     	i := 1
     	for i < (len(L) - 1) && CompareID(L[i].ID, cChar.ID) {
     		i += 1
     	}
     	// make recursive call using updated prev/next WChars
     	doc.IntegrateIns(cChar, L[i-1], L[i])
	}
}

func BroadcastOperation(op *Operation) {
	for _, replica := range activeReplicasMap {
		if replica.NodeId != replicaID {
			r, err := rpc.Dial("tcp", replica.RPCAddr)
			if err != nil {
				fmt.Println("Cannot contact replica")
				continue
			}

			var result ValReply

			err = r.Call("ReplicaService.ReceiveOperation", op, &result)
			if err != nil {
				fmt.Println("Cannot call replica procedure")	
			} 
		}
	}
}

// receive operation from remote replica and add to op pool
func (rs *ReplicaService) ReceiveOperation(receivedOp *Operation, reply *ValReply) error {
	fmt.Println("Receiving op: " + receivedOp.OpType)

	// Add operation to the correct pool
	if replicaID == storageFlag {
		storageOpPool = append(storageOpPool, receivedOp)
	} else {
	
		if document == nil {
			fmt.Println("document is nil")
			return nil
		}
	
		document.opPool = append(document.opPool, receivedOp)
	}

	vectorClocks := make(map[int]int)
	vectorClocks[receivedOp.OriginatorID] = receivedOp.OriginatorClock
	LogEventWithVectors("Received op: " + receivedOp.OpType, vectorClocks)

	reply.Val = ""
	reply.SenderID = replicaID
	reply.SenderClock = shivizClock

	return nil
}

// Process all pending operations
func ProcessOperations() {
	for {
		runtime.Gosched()		
		if replicaID == storageFlag { // if storage replica, loop through storage operation pool
			for i := len(storageOpPool) - 1; i >= 0; i-- { // use a downward loop, to avoid errors when removing elements while inside a loop
				op := storageOpPool[i]
				storedDocument, exists := documentsMap[op.DocumentId]
				if exists {
					if storedDocument.IsExecutable(op) {
						switch op.OpType {
							case "ins":
								prevCharacter := storedDocument.WCharDic[ConstructKeyFromID(op.OpChar.PrevID)]
								nextCharacter := storedDocument.WCharDic[ConstructKeyFromID(op.OpChar.NextID)]
								storedDocument.IntegrateIns(op.OpChar, prevCharacter, nextCharacter)
								storageOpPool = append(storageOpPool[:i], storageOpPool[i+1:]...) // remove from pool
							case "del": 
								 _ = storedDocument.IntegrateDel(op.Position)
								 storageOpPool = append(storageOpPool[:i], storageOpPool[i+1:]...)
						}
					}	
				}
			}
		} else { // loop through document pool
			if document == nil {
				continue
			}
			
			for i := len(document.opPool) - 1; i >= 0; i-- {
				op := document.opPool[i]
				if document.IsExecutable(op) {
					switch op.OpType {
						case "ins":
							prevChar := document.WCharDic[ConstructKeyFromID(op.OpChar.PrevID)]
							nextChar := document.WCharDic[ConstructKeyFromID(op.OpChar.NextID)]
							document.IntegrateIns(op.OpChar, prevChar, nextChar)
							if op.DocumentId == document.DocName {
								hub.broadcast <- []byte(constructString(document.WString))
							}
							document.opPool = append(document.opPool[:i], document.opPool[i+1:]...) // remove from pool
						case "del": 
							_ = document.IntegrateDel(op.Position)
							if op.DocumentId == document.DocName {
								hub.broadcast <- []byte(constructString(document.WString))
							}
							document.opPool = append(document.opPool[:i], document.opPool[i+1:]...)
					}
				}
			}
		}		
	}
}

// WOOT Helper Methods

// construct string from a document
func constructString(wString []*WCharacter) string {
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
func (doc *Document) Insert(char *WCharacter, p int) {
	temp := WCharacter{}
	doc.WString = append(doc.WString, &temp)
	copy(doc.WString[p+1:], doc.WString[p:])
	doc.WString[p] = char
	
	// also add to WCharDic
	doc.WCharDic[ConstructKeyFromID(char.ID)] = char	

	docString := constructString(doc.WString)
	fmt.Printf("Current WString: %s\n", docString)
}

// get subsequence of wstring between prevchar and nextchar
func (doc *Document) Subsequence(prevChar WCharacter, nextChar WCharacter) []*WCharacter {
	subseq := make([]*WCharacter, 0)
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
func (doc *Document) getIthVisible(i int) (*WCharacter, bool) {
	index := 0

	for _, wChar := range doc.WString { // TODO: check termination conditions
		if index == i && wChar.IsVisible { // found ith visible
			return wChar, true
		} else if wChar.IsVisible{ // current character is not visible, don't increment index
			index += 1
		} 
	}
	return nil, false // no ith visible character
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

func ConstructKeyFromID(ID []int) string {
	return (strconv.Itoa(ID[0]) + "-" + strconv.Itoa(ID[1]))
}

func isStartOrEndChar(wChar *WCharacter) bool {
	if (wChar.ID[0] == startChar.ID[0] && wChar.ID[1] == startChar.ID[1]) || (wChar.ID[0] == endChar.ID[0] && wChar.ID[1] == endChar.ID[1]) {
		fmt.Println("trying to delete start or end char!")
		return true
	}
	return false
}

func (doc *Document) printWCharDic() {
	for _, entry := range doc.WCharDic {
		fmt.Println(entry)
	}
}


// Shiviz logging helper functions

func LogLocalEvent(event string) {
	// Clean up strings
	event = strings.TrimSpace(event)

	// Increment shiviz clock
	shivizClock += 1

	shivizEvent := ShivizEvent{
		Event: event,
		HostID: replicaID,
		VectorClocks: make(map[int]int)}
	shivizEvent.VectorClocks[replicaID] = shivizClock

	LogEvent(shivizEvent)

	// Send event to all replicas so that they can log the event themselves
	for _, replica := range activeReplicasMap {
		if replica.NodeId != replicaID {
			r, err := rpc.Dial("tcp", replica.RPCAddr)
			if err != nil {
				fmt.Println("Cannot contact replica")
			}

			var result ValReply

			err = r.Call("ReplicaService.ReceiveRemoteEvent", &shivizEvent, &result)
			if err != nil {
				fmt.Println("Cannot call replica procedure")	
			}
		}
	}
}

func LogEventWithVectors(event string, vectorClocks map[int]int) {
	// Increment shiviz clock
	shivizClock += 1

	shivizEvent := ShivizEvent{
		Event: event,
		HostID: replicaID,
		VectorClocks: make(map[int]int)}
	shivizEvent.VectorClocks = vectorClocks
	shivizEvent.VectorClocks[replicaID] = shivizClock

	LogEvent(shivizEvent)

	// Send event to all replicas so that they can log the event themselves
	for _, replica := range activeReplicasMap {
		if replica.NodeId != replicaID {
			r, err := rpc.Dial("tcp", replica.RPCAddr)
			if err != nil {
				fmt.Println("Cannot contact replica")
			}

			var result ValReply

			err = r.Call("ReplicaService.ReceiveRemoteEvent", &shivizEvent, &result)
			if err != nil {
				fmt.Println("Cannot call replica procedure")	
			}
		}
	}

}

func LogEvent(event ShivizEvent) {
	if !Logger.LogThis(event.Event, strconv.Itoa(event.HostID), ConstructClockString(event.VectorClocks)) {
		fmt.Println("Failed to log event!")
	}
}

// receive event from remote replica and log it
func (rs *ReplicaService) ReceiveRemoteEvent(receivedEvent *ShivizEvent, reply *ValReply) error {
	fmt.Println("Receiving event: \"" + receivedEvent.Event + "\"")

	reply.Val = ""

	LogEvent(*receivedEvent)

	return nil
}

func ConstructClockString(clockMap map[int]int) string {
	clockString := "{"
	
	for hostID, hostClock := range clockMap {
		clockString += "\"" + strconv.Itoa(hostID) + "\":" + strconv.Itoa(hostClock) + ", "
	}
	clockString = strings.TrimSuffix(clockString, ", ")	
	clockString += "}"

	return clockString
}

// If error is non-nil, print it out and halt.
func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error ", err.Error())
		os.Exit(1)
	}
}

/*
// Tests
func runTests() {
	fmt.Println("Starting testing")

	testDoc := Document {
		DocName: "testDoc",
		WString: []*WCharacter{&startChar, &endChar}, // intialize to empty, only special chars exist
		WCharDic: make(map[string]*WCharacter), 
		opPool: []*Operation{} }

	// add special chars to WCharDic
	testDoc.WCharDic[ConstructKeyFromID(startChar.ID)] = &startChar
	testDoc.WCharDic[ConstructKeyFromID(endChar.ID)] = &endChar

	//posTests(&testDoc)

	// reset testDoc
	testDoc = Document {
		DocName: "testDoc",
		WString: []*WCharacter{&startChar, &endChar}, // intialize to empty, only special chars exist
		WCharDic: make(map[string]*WCharacter), 
		opPool: []*Operation{} }

	// add special chars to WCharDic
	testDoc.WCharDic[ConstructKeyFromID(startChar.ID)] = &startChar
	testDoc.WCharDic[ConstructKeyFromID(endChar.ID)] = &endChar

	IntegrateTests(&testDoc)
}

func posTests(doc *Document) {

	// single character
	replicaClock += 1
	char1 := WCharacter{
		ID: []int{replicaID, replicaClock},
		IsVisible: true, 
		CharVal: "a",
		PrevID: []int{startChar.ID[0], startChar.ID[1]},
		NextID: []int{endChar.ID[0], endChar.ID[1]} }

	replicaClock += 1
	char2 := WCharacter{
		ID: []int{replicaID, replicaClock},
		IsVisible: true, 
		CharVal: "b",
		PrevID: []int{char1.ID[0], char1.ID[1]},
		NextID: []int{endChar.ID[0], endChar.ID[1]} }

	replicaClock += 1
	char3 := WCharacter{
		ID: []int{replicaID, replicaClock},
		IsVisible: true, 
		CharVal: "c",
		PrevID: []int{startChar.ID[0], startChar.ID[1]},
		NextID: []int{endChar.ID[0], endChar.ID[1]} }

	doc.WString = []*WCharacter{&startChar, &char1, &char2, &endChar}
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
	doc.Insert(&char3, 2)
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
	doc.Insert(&char4, 4)
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
	doc.Insert(&char5, 2)
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

func IntegrateTests(doc *Document) {

	replicaClock += 1
	char1 := WCharacter{
		ID: []int{replicaID, replicaClock},
		IsVisible: true, 
		CharVal: "a",
		PrevID: []int{startChar.ID[0], startChar.ID[1]},
		NextID: []int{endChar.ID[0], endChar.ID[1]} }

	fmt.Println("IntegrateInsert for a between start and end")
	doc.IntegrateIns(&char1, &startChar, &endChar)
	docString := constructString(doc.WString)
	fmt.Printf("Current WString: %s\n", docString)


	replicaClock += 1
	char2 := WCharacter{
		ID: []int{replicaID, replicaClock},
		IsVisible: true, 
		CharVal: "b",
		PrevID: []int{char1.ID[0], char1.ID[1]},
		NextID: []int{endChar.ID[0], endChar.ID[1]} }

	fmt.Println("IntegrateInsert for b between start and end")
	doc.IntegrateIns(&char2, &startChar, &endChar)
	docString = constructString(doc.WString)
	fmt.Printf("Current WString: %s\n", docString)

	replicaClock += 1
	char3 := WCharacter{
		ID: []int{replicaID, replicaClock},
		IsVisible: true, 
		CharVal: "c",
		PrevID: []int{startChar.ID[0], startChar.ID[1]},
		NextID: []int{endChar.ID[0], endChar.ID[1]} }

	fmt.Println("IntegrateInsert for c between start and b")
	doc.IntegrateIns(&char3, &startChar, &char2)
	docString = constructString(doc.WString)
	fmt.Printf("Current WString: %s\n", docString)

	fmt.Println("IntegrateDel for b")
	doc.IntegrateDel(&char2)
	docString = constructString(doc.WString)
	fmt.Printf("Current WString: %s\n", docString)
}*/