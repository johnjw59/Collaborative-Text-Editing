angular.module('app', ['eb.caret'])

.controller('main', function ($scope) {
	var replica = new WebSocket('ws://localhost:8080/ws');

	var pathname = window.location.pathname;
	var initNewDoc = false;

	if (pathname == "/") {
		initNewDoc = true;
	}

	// Get initial copy of document
	replica.onopen = function () {

		if (initNewDoc) { // Create a new document link
			replica.send(JSON.stringify({
					'op' : 'init'
				}));
		} else { // Retrieve a document based on the specified document id given in the URL path
			var paths = pathname.split("/");
			if (paths.length == 3 && paths[1] == "doc") {
				var documentId = paths[2];
				replica.send(JSON.stringify({
						'op' : 'retrieve',
						'val' : documentId
					}));
			}
		}
	};

	// Integrate received document
	replica.onmessage = function (event) {
		
		if (initNewDoc) {
			$scope.shareLink = "Share Link: localhost:8080/doc/" + event.data;
			$scope.$apply();
			isNewDoc = false;
		} else {
			console.log(event);
			$scope.document = event.data;
			$scope.$apply();	
		}
	};

	// Document has been edited. Send changes.
	$scope.documentEdit = function (event) {
		// Keypress events give text.
		if (event.type == 'keypress') {
			// Send insertion
			replica.send(JSON.stringify({
					'op' : 'ins',
					'val' : String.fromCharCode(event.charCode),
					'pos' : $scope.cursor.get
				}));

			// Check keyup for Backspace/Delete
		} else if (event.type == 'keyup') {
			// Send deletion
			if (event.keyCode == 8) {
				// Backspace
				replica.send(JSON.stringify({
						'op' : 'del',
						'pos' : $scope.cursor.get
					}));
			} else if (event.keyCode == 46) {
				// Delete
				replica.send(JSON.stringify({
						'op' : 'del',
						'pos' : $scope.cursor.get + 1
					}));
			}
		}
	};

});
