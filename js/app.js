angular.module('app', ['angular-json-rpc', 'eb.caret'])

.controller('main', function($scope, $http, $q) {
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
    $scope.$broadcast('replica_ip', event.data);
  };

  $scope.$on('replica_ip', function(e, address) {
    console.log('got a new replica IP');
    $http.jsonrpc('http://' + address +'/rpc', 'ReplicaService.ReadFromDoc', [{}])
    .success(function(data, status, headers, config) {
      console.log('RPC call success!');
      console.log(data);
    }).error(function(data, status, headers, config) {
      console.error('RPC call failed - ' + status);
      console.log(config);
    });
  });

});
