angular.module('app', ['eb.caret'])

.controller('main', function($scope, $http, $q) {
  var ws = new WebSocket('ws://localhost:8080/ws');
  ws.onopen = function() {
    ws.send('ping');
  };
  ws.onclose = function() {
    console.log('websocket closed');
  };
  ws.onmessage = function(event) {
    console.log(event.data);
  };

});
