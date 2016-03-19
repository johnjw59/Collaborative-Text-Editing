angular.module('app', ['angular-jsonrpc-client', 'eb.caret'])
.controller('main', function($scope, jsonrpc) {
  var ws = new WebSocket('ws://localhost:8080/route');
  ws.onopen = function() {
    ws.send('ping');
  };
  ws.onclose = function() {
    console.log('websocket closed');
  };
  ws.onmessage = function(event) {
    $scope.replica_ip = event.data;
    $scope.$apply();
  };


});
