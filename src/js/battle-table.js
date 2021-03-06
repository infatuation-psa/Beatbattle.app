function embed(button) {
  /* Todo - width */
  embedUrl = button.data("embed")
  if(getHostnameFromRegex(embedUrl) == "soundcloud.com") {
      console.log("soundcloud")
      var urlSplit = embedUrl.split("/")
      console.log(urlSplit)

      embedData = `<iframe height='20' scrolling='no' frameborder='no' allow='autoplay' src='https://w.soundcloud.com/player/?url=`
      // If secret URL
      if(urlSplit.length >= 6) {
          embedUrl = "https://soundcloud.com/" + urlSplit[3] + "/" + urlSplit[4] + `?secret_token=` + urlSplit[5]
      }

      embedData += embedUrl
      embedData += `&color=%23ff5500&inverse=true&auto_play=true&show_user=false'></iframe>`
      var toembed = button.closest(".embedded-track");
      toembed.html(embedData);
  } else if(getHostnameFromRegex(embedUrl) == "audius.co") {

  }
}

const getHostnameFromRegex = (url) => {
  // run against regex
  const matches = url.match(/^https?\:\/\/([^\/?#]+)(?:[\/?#]|$)/i);
  // extract hostname (will be null if no match is found)
  return matches && matches[1];
}

function onChange() {
    // AJAX should be changed to match the other ajax form.
    $(".tooltipped").tooltip();
      
    $(".playButton").click(function () {
      var button = $(this);
      embed(button)
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

      $scope.drawTable = true;
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
        event.stopPropagation();
      
        function updateCascade(beat) {
          var sortedBeats = JSON.parse(JSON.stringify($scope.beats.data));
          sortedBeats.splice(beat.index, 1);
          sortedBeats.splice(beat.placement-1, 0, beat)
          
          for(var i = 0; i < sortedBeats.length; i++) {
              if(sortedBeats[i].voted == 1) {
                $scope.beats.data[sortedBeats[i].index].placement = i + 1;
              }
          }
          $scope.refreshTable();
        }
        
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
                  }));
                  
                  if(t.ToastQuery == "placement") {
                    updateCascade(beat);
                  }
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

      $scope.refreshTable = function() {
        var beats = JSON.parse(JSON.stringify($scope.beats.data));
        $scope.beats.data = []
        $timeout(function () {
          $scope.beats.data = beats
        }, 50);
      }
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