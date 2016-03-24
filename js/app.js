angular.module('app', ['eb.caret'])

.controller('main', function($scope, $http, $q) {
  var ws = new WebSocket('ws://localhost:8080/ws');

  /*ws.onopen = function() {
    ws.send('ping');
    // Get initial copy of document
  };*/

  ws.onmessage = function(event) {
    console.log(event.data);
    // integrate recieved document
  };

  // Document has been edited. Send changes.
  $scope.documentEdit = function(event) {
    console.log(event);

    // Check if key pressed is text, backspace, delete or enter (ignore others)
    if ((event.keyCode >= 32 && event.keyCode <= 136) || event.keyCode == 13) {
      var char = String.fromCharCode(event.keyCode);
      if (!event.shiftKey) {
        char = char.toLowerCase();
      }
      // Send char to Replica
      ws.send(JSON.stringify({"op": "ins", "val": char, "pos": 0}));
    } 
    else if (event.keyCode == 8) {
      // send backspace
      console.log("Backspace");
    } 
    else if (event.keyCode == 127) {
      // send delete
      console.log("Delete");
    }

  };

});
