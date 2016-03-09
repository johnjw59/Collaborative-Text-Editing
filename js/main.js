$(document).ready(function() {
	
	var websocket = new WebSocket("ws://localhost:8080/route");	
	websocket.onopen = function(event) {
		console.log("opened");
		// Ping the websocket
		websocket.send("ping")
	}
	websocket.onclose = function(event) {
		console.log("closed");
	}
	websocket.onmessage = function(event) {
		$("#rpc_address").text("Assigned replica RPC Address: " + event.data)
	}
	
});