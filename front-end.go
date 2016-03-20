package main

import (
	"fmt"
	"os"
	"net"
	"encoding/json"
	"flag"
	"net/http"
	"text/template"
	"log"
	"bytes"
	"github.com/gorilla/websocket"
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

var homeTemplate = template.Must(template.ParseFiles("home.html"))
var upgrader = websocket.Upgrader{} // use default options
var httpAddress = flag.String("addr", ":8080", "http service address")

// Main server loop.
func main() {
	// Parse args.
	usage := fmt.Sprintf("Usage: %s [client ip:port] [replica-node ip:port]\n",
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
	
	go ReplicaListener(replicaConn)
	
	flag.Parse()
	// start HTTP server
	http.Handle("/", http.StripPrefix("/", http.FileServer(http.Dir("./"))))
	http.HandleFunc("/route", RouteClient)
	http.HandleFunc("/home", ServeHome)
	err = http.ListenAndServe(*httpAddress, nil)
	if err != nil {
		log.Fatal("Error on HTTP serve: ", err)
	}
}

// Send a replica's RPCAddress to client to start communication with it
func RouteClient(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()
	for {
		mt, _, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		
		// Randomly pick a replica to route to (note that this is random since the Go runtime randomizes map iteration order) 
		var addressToSend string
		for nodeId, RPCAddress := range replicaRPCMap {
			
			// Test availability by dialing to the TCP/RPC address
			_, err := net.Dial("tcp", RPCAddress)
			if err != nil {
				delete(replicaRPCMap, nodeId)
				UpdateReplicas()
			} else {
				addressToSend = RPCAddress
				break	
			}
		}
		
		err = c.WriteMessage(mt, []byte(addressToSend))
		if err != nil {
			log.Println("write:", err)
			break
		}
	}
}

// Serve the home page at localhost:8080
func ServeHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not found", 404)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	homeTemplate.Execute(w, "ws://" + r.Host + "/route")
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
