angular
  .module("BeatBattle", ["ngMaterial", "md.data.table"])

  .config([
    "$mdThemingProvider",
    function ($mdThemingProvider) { 
      "use strict";
      $mdThemingProvider.theme("default");
    },
  ])

  .controller("BeatBattleController", [
    "$mdEditDialog",
    "$q",
    "$scope",
    "$timeout",
    function ($mdEditDialog, $q, $scope, $timeout) {
      "use strict";

      $scope.selected = [];
      $scope.limitOptions = [10, 25, 100];

      $scope.query = {
        order: "name",
        limit: 10,
        page: 1
      };

      $scope.feedback = {
        count: feedback.length,
        data: feedback
      };

      $scope.toggleLimitOptions = function () {
        $scope.limitOptions = $scope.limitOptions ? undefined : [10, 25, 100];
      };

      $scope.logOrder = function (order) {
        console.log("order: ", order);
      };

      $scope.tableChange = function () {
        console.log("changed");
      };
    }
  ]);

$(document).ready(function() {   
    $('.deadline').each(function(index, obj){
        $(this).countdown($(this).attr("deadline"), function(event) {
            $(this).text(
                event.strftime('%Dd %Hh %Mm %Ss')
            );
        });
    });
})      