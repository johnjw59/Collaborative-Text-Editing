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
	"github.com/gorilla/websocket"
)

type Replica struct {
	NodeId  string
	RPCAddr string
}

// map to where the key is the replica id and the value is the replica's IP
var replicaMap map[string]string

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
	
	replicaMap = make(map[string]string)

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
		
		// naively get the first replica's address 
		// TODO: Add logic to test availability of replica and remove the replica from map
		var addressToSend string
		for _, RPCAddress := range replicaMap {
			addressToSend = RPCAddress
			break
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
			replicaMap[replica.NodeId] = replica.RPCAddr
		} else {
			checkError(err)
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
