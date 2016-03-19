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
  };*/

  //$scope.$on('replica_ip', function(e, address) {
    console.log('got a new replica IP');
    $http.jsonrpc('http://localhost:5050/rpc', 'ReplicaService.WriteToDoc', [{'newString': '1234'}])
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

    /*var request = new XMLHttpRequest();

    var command = {
       "jsonrpc":"2.0",
       "method":"ReplicaService.ReadFromDoc",
       "params":[{
           'newString': '1234'
       }],
       "id":1
    };

    request.onreadystatechange = function() {
      if (request.readyState == 4 && request.status == 200) {
          var response = JSON.parse(request.responseText).result;
          console.log(response);
      } else {
          console.log(request);
      }
    };

    request.open("POST", 'http://' + address + '/rpc', true);
    request.setRequestHeader("Content-Type", "application/json;charset=UTF-8");
    request.send(JSON.stringify(command));*/

  //});

});
