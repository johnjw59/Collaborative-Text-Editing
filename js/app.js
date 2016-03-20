angular.module('app', ['angular-json-rpc', 'eb.caret'])

.controller('main', function($scope, $http, $q) {
  /*var ws = new WebSocket('ws://localhost:8080/route');
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
    console.log('got a new replica IP');*/
    $http.jsonrpc('http://localhost:5050/rpc', 'Service.DoSomething', 
      [{
        "Str":"pie",
        "Val":3.14
      }]
    )
    .success(function(data, status, headers, config) {
      console.log('it was a success?');
      console.log(data);
      console.log(status);
      console.log(headers);
      console.log(config);
    }).error(function(data, status, headers, config) {
      console.log('failure!');
      console.log(data);
      console.log(status);
      console.log(headers);
      console.log(config);
    });
  //});

});
