package main

import (
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
)

// GetToast serves toast text.
func GetToast(c echo.Context) [2]string {
	html := ""
	class := ""

	sess, _ := session.Get("beatbattle", c)
	errorCode := sess.Values["error"]

	switch message := errorCode; message {
	case "404":
		html = "Requested resource not found."
		class = "toast-error"
	case "502":
		html = "Server error."
		class = "toast-error"
	case "password":
		html = "Incorrect password."
		class = "toast-error"
	case "unapprovedurl":
		html = "URL not on approved list."
		class = "toast-error"
	case "notopen":
		html = "That battle is not currently open."
		class = "toast-error"
	case "nobeat":
		html = "You haven't submitted a beat to this battle."
		class = "toast-error"
	case "noauth":
		html = "You need to be logged in to do that."
		class = "toast-error"
	case "notuser":
		html = "You're not allowed to do that."
		class = "toast-error"
	case "notvoting":
		html = "This battle isn't currently accepting votes."
		class = "toast-error"
	case "owntrack":
		html = "You can't vote for your own track."
		class = "toast-error"
	case "maxvotes":
		html = "You're at your max votes for this battle."
		class = "toast-error"
	case "deadb4":
		html = "The deadline cannot be before right now."
		class = "toast-error"
	case "voteb4":
		html = "The voting deadline cannot be before the deadline."
		class = "toast-error"
	case "maxvotesinvalid":
		html = "Max votes must be between 1 and 10."
		class = "toast-error"
	case "nodata":
		html = "No data received.."
		class = "toast-error"
	case "validationerror":
		html = "Validation error, please try again."
		class = "toast-error"
	case "maxbattles":
		html = "You can only have 3 active battles at once."
		class = "toast-error"
	case "titleexists":
		html = "You already have a battle with this title."
		class = "toast-error"
	case "sconly":
		html = "You must submit a SoundCloud link."
		class = "toast-error"
	case "cache":
	case "cachesave":
		html = "If this happens again, try clearing your cache."
		class = "toast-error"
	case "feedbackself":
		html = "You can't give yourself feedback."
		class = "toast-error"
	case "invalidtype":
		html = "That is not a valid battle type."
		class = "toast-error"
	case "liked":
		html = "Submission loved."
		class = "toast-success"
	case "unliked":
		html = "Submission unloved."
		class = "toast-success"
	case "successvote":
		html = "Vote successful."
		class = "toast-success"
	case "successdelvote":
		html = "Vote successfully removed."
		class = "toast-success"
	case "successdel":
		html = "Successfully deleted."
		class = "toast-success"
	case "successadd":
		html = "Successfully added."
		class = "toast-success"
	case "successupdate":
		html = "Successfully updated."
		class = "toast-success"
	case "successaddfeedback":
		html = "Successfully added feedback."
		class = "toast-success"
	case "acceptreq":
		html = "User has been added to the group."
		class = "toast-success"
	case "accept":
		html = "Successfully joined group."
		class = "toast-success"
	case "successreq":
		html = "Requested to join group."
		class = "toast-success"
	case "declinereq":
		html = "User has not been added to the group."
		class = "toast-success"
	case "decline":
		html = "Successfully declined invite."
		class = "toast-success"
	case "successinv":
		html = "Successfully invited user."
		class = "toast-success"
	case "ingroupreq":
		html = "User already in group."
		class = "toast-error"
	case "ingroup":
		html = "You're already in the group."
		class = "toast-error"
	case "notingroup":
		html = "Not in group."
		class = "toast-error"
	case "invalid":
		html = "Your SoundCloud url format is invalid."
		class = "toast-error"
	case "reqexists":
		html = "You've already requested to join this group."
		class = "toast-error"
	case "invexists":
		html = "User has already been invited to the group."
		class = "toast-error"
	case "notopengrp":
		html = "This group is not open."
		class = "toast-error"
	case "relog":
		html = "Login session expired."
		class = "toast-error"
	}

	sess.Values["error"] = ""
	sess.Save(c.Request(), c.Response())

	if html != "" {
		return [2]string{html, class}
	}

	return [2]string{}
}

// SetToast serves toast text.
func SetToast(c echo.Context, code string) {
	sess, _ := session.Get("beatbattle", c)
	sess.Values["error"] = code
	sess.Save(c.Request(), c.Response())
}
