angular.module('app', ['eb.caret'])

.controller('main', function($scope) {
  var replica = new WebSocket('replica://localhost:8080/replica');

  // Get initial copy of document
  replica.onopen = function() {
    replica.send(JSON.stringify({'op': 'init'}));
  };

  // Integrate recieved document
  replica.onmessage = function(event) {
    console.log(event);
    $scope.document = event.data;
    $scope.$apply();
  };

  // Document has been edited. Send changes.
  $scope.documentEdit = function(event) {
    // Keypress events give text.
    if (event.type == 'keypress') {
      // Send insertion
      replica.send(JSON.stringify({'op': 'ins', 'val': String.fromCharCode(event.charCode), 'pos': $scope.cursor.get}));

    // Check keyup for Backspace/Delete
    } else if (event.type == 'keyup') {
      // Send deletion
      if (event.keyCode == 8) {
        // Backspace
        replica.send(JSON.stringify({'op': 'del', 'pos': $scope.cursor.get -1}));
      } else if (event.keyCode == 46) {
        // Delete
        replica.send(JSON.stringify({'op': 'del', 'pos': $scope.cursor.get}));
      }
    }
  };

});
