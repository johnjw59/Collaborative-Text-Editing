package main

import (
	"fmt"
	"os"
	"net"
	"net/rpc"
	"encoding/json"
	"time"
	"flag"
	"net/http"
	"text/template"
	"log"
)

type Replica struct {
	NodeId  string
	RPCAddr string
}

// map to where the key is the replica id and the value is the replica's IP
var replicaMap map[string]string

var httpAddress = flag.String("addr", ":8080", "http service address")
var homeTemplate = template.Must(template.ParseFiles("home.html"))

// Main server loop.
func main() {
	// Parse args.
	usage := fmt.Sprintf("Usage: %s [client ip:port] [replica-node ip:port]\n",
		os.Args[0])
	if len(os.Args) != 3 {
		fmt.Printf(usage)
		os.Exit(1)
	}
	
	replicaMap = make(map[string]string)

	clientAddrString := os.Args[1]
	
	replicaAddrString := os.Args[2]
	replicaAddr, err := net.ResolveUDPAddr("udp", replicaAddrString)
	checkError(err)
	
	// start UDP server to listen for replica node activity
	replicaConn, err := net.ListenUDP("udp", replicaAddr)
	if err != nil {
		fmt.Println("Error on UDP listen: ", err)
		os.Exit(-1)
	}
	
	go ReplicaListener(replicaConn)
	// go ReplicaActivityListener()
	
	// start TCP server to listen for new clients
	clientListener, err := net.Listen("tcp", clientAddrString)
	if err != nil {
		fmt.Println("Error on TCP listen: ", err)
		os.Exit(-1)
	}
	
	flag.Parse()
	// start HTTP server
	go func() {
		http.HandleFunc("/", ServeHome)
		err = http.ListenAndServe(*httpAddress, nil)
		if err != nil {
			log.Fatal("Error on HTTP serve: ", err)
		}
	}()
	
	for {
		clientConn, err := clientListener.Accept()
		checkError(err)
		go RouteClient(clientConn)		
	}
}

// serve the home page at localhost:8080
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
	homeTemplate.Execute(w, r.Host)
}

// Send a replica's RPCAddress to client to start communication with it
func RouteClient(conn net.Conn) {

	for _, RPCAddress := range replicaMap {
		conn.Write([]byte(RPCAddress))
		conn.Close()	
		break
	}
}

// Periodically check each node for availability (every second)
func ReplicaActivityListener() {

	for {
		for nodeId, RPCaddress := range replicaMap {
			_, err := rpc.Dial("tcp", RPCaddress)
			if err != nil {
				// remove the replica in the map
				fmt.Println("Removed replica: " + nodeId)
				delete(replicaMap, nodeId)
			}
		}
		
		time.Sleep(1000 * time.Millisecond)
	}
}

// Function to listen for newly connected replica nodes
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
