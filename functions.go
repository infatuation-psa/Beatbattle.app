package main

import (
	"fmt"
	"log"
	"math/rand"

	"github.com/labstack/echo/v4"
)

// GetToast serves toast text.
func GetToast(c echo.Context) [2]string {
	html := ""
	class := ""

	// Get session
	sess, err := store.Get(c.Request(), "beatbattleapp")
	if err != nil {
		fmt.Println(fmt.Sprintf("Session get err: %s", err))
	}

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
	case "notvoting":
		html = "This battle isn't currently accepting votes."
		class = "toast-error"
	case "owntrack":
		html = "You can't vote for your own track."
		class = "toast-error"
	case "maxvotes":
		html = "You're at your max votes for this battle."
		class = "toast-error"
	case "voteb4":
		html = "The voting deadline cannot be before the deadline."
		class = "toast-error"
	case "validationerror":
		html = "Validation error, please try again."
		class = "toast-error"
	case "maxbattles":
		html = "You can only have 3 active battles at once."
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
	case "successclose":
		html = "Successfully closed."
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
	case "invalid":
		html = "Your SoundCloud url format is invalid."
		class = "toast-error"
	case "relog":
		html = "Login session expired."
		class = "toast-error"
	case "requalified":
		html = "Requalified submission."
		class = "toast-success"
	case "disqualified":
		html = "Disqualified submission."
		class = "toast-success"
	case "403":
		html = "User lacks permissions."
		class = "toast-error"
	case "placement":
		html = "Changed placement."
		class = "toast-success"
	}

	sess.Values["error"] = ""
	sess.Save(c.Request(), c.Response())

	if html != "" {
		return [2]string{html, class}
	}

	return [2]string{}
}

// GetAdvertisements returns an array of the current active ads.
// TODO - Can store this in cache?
func GetAdvertisements() Advertisement {
	query := `SELECT id, url, image FROM ads WHERE active = 1`

	rows, err := dbRead.Query(query)
	if err != nil {
		return Advertisement{}
	}
	defer rows.Close()

	advertisement := Advertisement{}
	advertisements := []Advertisement{}
	for rows.Next() {
		err = rows.Scan(&advertisement.ID, &advertisement.URL, &advertisement.Image)
		if err != nil {
			return Advertisement{}
		}

		advertisements = append(advertisements, advertisement)
	}

	// Reference: http://go-database-sql.org/errors.html - I'm not really sure if this does anything positive lmao.
	if err = rows.Err(); err != nil {
		log.Println(err)
	}
	if err = rows.Close(); err != nil {
		log.Println(err)
	}

	if len(advertisements) > 0 {
		randomIndex := rand.Intn(len(advertisements))
		return advertisements[randomIndex]
	}

	return Advertisement{}
}

// SetToast serves toast text.
func SetToast(c echo.Context, code string) {
	sess, err := store.Get(c.Request(), "beatbattleapp")
	if err != nil {
		fmt.Println(fmt.Sprintf("(TOAST) Session get err: %s", err))
	}
	sess.Values["error"] = code
	sess.Save(c.Request(), c.Response())
}
