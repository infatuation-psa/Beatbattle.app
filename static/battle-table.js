

function onChange() {
    // AJAX should be changed to match the other ajax form.
    $(".tooltipped").tooltip();
      
    $(".playButton").click(function () {
      var button = $(this);
      embed = button.data("embed");
      var toembed = button.closest(".embedded-track");
      toembed.html(embed);
    });
}

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

      $scope.beats = {
        count: battleEntries.length,
        data: battleEntries
      };

      $scope.toggleLimitOptions = function () {
        $scope.limitOptions = $scope.limitOptions ? undefined : [10, 25, 100];
      };

      $scope.editPlacement = function (event, beat) {
        // if auto selection is enabled you will want to stop the event
        // from propagating and selecting the row
        event.stopPropagation();
        
        /* 
        * messages is commented out because there is a bug currently
        * with ngRepeat and ngMessages were the messages are always
        * displayed even if the error property on the ngModelController
        * is not set, I've included it anyway so you get the idea
        */
        
        var promise = $mdEditDialog.small({
          modelValue: beat.placement,
          save: function (input) {
            beat.placement = parseInt(input.$modelValue);
            $.ajax({
                "url": "/placement",
                "data": "battleID=" + beat.battle_id + "&beatID=" + beat.id + "&placement=" + beat.placement,
                "type": "post",
                "success": function(t) {
                  t.Redirect ? window.location.replace(t.RedirectPath) : (M.toast({
                      html: t.ToastHTML,
                      classes: t.ToastClass,
                      displayLength: 1500,
                  }))
                }
            });
          },
          targetEvent: event,
          validators: {
            'md-maxlength': 4
          }
        });

        promise.then(function (ctrl) {
          var input = ctrl.getInput();

          input.$viewChangeListeners.push(function () {
            input.$setValidity('test', input.$modelValue !== 'test');
          });
        });
      };

      $scope.editFeedback = function (event, beat) {
        // if auto selection is enabled you will want to stop the event
        // from propagating and selecting the row
        event.stopPropagation();
        
        /* 
        * messages is commented out because there is a bug currently
        * with ngRepeat and ngMessages were the messages are always
        * displayed even if the error property on the ngModelController
        * is not set, I've included it anyway so you get the idea
        */

        var promise = $mdEditDialog.small({
          modelValue: beat.feedback,
          placeholder: 'Add feedback',
          save: function (input) {
            $.ajax({
                "url": "/feedback",
                "data": "beatID=" + beat.id + "&feedback=" + input.$modelValue,
                "type": "post",
                "success": function(t) {
                  t.Redirect ? window.location.replace(t.RedirectPath) : (M.toast({
                      html: t.ToastHTML,
                      classes: t.ToastClass,
                      displayLength: 1500,
                  }))
                }
            });
            battleEntries[beat.index].feedback = input.$modelValue;
          },
          targetEvent: event,
          validators: {
            'md-maxlength': 256
          }
        });

        promise.then(function (ctrl) {
          var input = ctrl.getInput();

          input.$viewChangeListeners.push(function () {
            input.$setValidity('test', input.$modelValue !== 'test');
          });
        });
      };

      $scope.likeBeat = function (event, beat) {
        event.stopPropagation();
        if(beat.user_like == 1) {
            battleEntries[beat.index].user_like = 0;
        } else {
            battleEntries[beat.index].user_like = 1;
        }
        
        $.ajax({
            "url": "/like",
            "data": "beatID=" + beat.id + "&battleID=" + beat.battle_id + "&userID=" + beat.artist.id,
            "type": "post",
            "success": function(t) {
                t.Redirect ? window.location.replace(t.RedirectPath) : (M.toast({
                    html: t.ToastHTML,
                    classes: t.ToastClass,
                    displayLength: 1500,
                }));    
            } 
        });
      };

      $scope.voteBeat = function (event, beat) {
        event.stopPropagation();
        console.log(beat.artist.id)
        if(beat.user_vote == 1) {
            battleEntries[beat.index].user_vote = 0;
        } else if(beat.user_vote == 0 && votesRemaining > 0) {
            battleEntries[beat.index].user_vote = 1;
        }

        $.ajax({
            "url": "/vote",
            "data": "beatID=" + beat.id + "&battleID=" + beat.battle_id + "&userID=" + beat.artist.id,
            "type": "post",
            "success": function(t) {
                t.Redirect ? window.location.replace(t.RedirectPath) : (M.toast({
                    html: t.ToastHTML,
                    classes: t.ToastClass,
                    displayLength: 1500,
                }));    
            if(t.ToastQuery == "successvote") {
                votesRemaining -= 1;
                $(".votes-remaining").html(votesRemaining);
              }
              if(t.ToastQuery == "successdelvote") {
                votesRemaining += 1;
                $(".votes-remaining").html(votesRemaining);
              }
            } 
        });
      };

      $scope.disqualifyBeat = function (event, beat) {
        event.stopPropagation();

        battleEntries[beat.index].voted = !beat.voted
        battleEntries[beat.index].placement = 999

        $.ajax({
            "url": "/disqualify",
            "data": "beatID=" + beat.id + "&battleID=" + beat.battle_id,
            "type": "post",
            "success": function(t) {
                t.Redirect ? window.location.replace(t.RedirectPath) : (M.toast({
                    html: t.ToastHTML,
                    classes: t.ToastClass,
                    displayLength: 1500,
                }));    
            if(t.ToastQuery == "disqualified") {
                i.attr("style", "color: #ff5800");
              }
              if(t.ToastQuery == "requalified") {
                i.attr("style", "");
              }
            } 
        });
      };
      

      $scope.logOrder = function (order) {
        console.log("order: ", order);
      };

      $scope.tableChange = function () {
        console.log("changed");
        onChange();
      };
    }
  ]);

$(document).ready(function() {   
    onChange();
    $('.deadline').each(function(index, obj){
        $(this).countdown($(this).attr("deadline"), function(event) {
            $(this).text(
                event.strftime('%Dd %Hh %Mm %Ss')
            );
        });
    });
})      