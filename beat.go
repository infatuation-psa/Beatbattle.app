package main

import (
	"net/http"
	"strconv"
	"strings"
)

// Beat struct.
type Beat struct {
	ID          int    `gorm:"column:id" json:"id"`
	Artist      string `json:"artist"`
	URL         string `gorm:"column:beat_url" json:"url"`
	Votes       int    `gorm:"column:votes" json:"votes"`
	ChallengeID int    `gorm:"column:challenge_id" json:"challenge_id,omitempty"`
	UserID      string `gorm:"column:user_id" json:"user_id,omitempty"`
	Color       string `json:"color"`
}

// SubmitBeat ...
func SubmitBeat(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	toast := GetToast(r.URL.Query().Get(":toast"))

	battleID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil {
		http.Redirect(w, r, "/", 301)
		return
	}

	// TODO - Change this GetBattle statement or change GetBattle, this doesn't need a * sql statement.

	battle := GetBattle(db, battleID)
	if battle.Title == "" {
		http.Redirect(w, r, "/", 301)
		return
	}

	var user = GetUser(w, r)

	m := map[string]interface{}{
		"Battle": battle,
		"User":   user,
		"Toast":  toast,
	}

	URL := r.URL.RequestURI()

	tpl := "SubmitBeat"
	if strings.Contains(URL, "update") {
		tpl = "UpdateBeat"
	}

	print(tpl)
	tmpl.ExecuteTemplate(w, tpl, m)
}

// InsertBeat ...
func InsertBeat(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	user := GetUser(w, r)
	if !user.Authenticated {
		http.Redirect(w, r, "/login/noauth", 301)
		return
	}

	battleID, err := strconv.Atoi(policy.Sanitize(r.URL.Query().Get(":id")))
	if err != nil {
		http.Redirect(w, r, "/", 301)
		return
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID)

	// TODO - BATTLE ID AND DEADLINE
	isOpen := RowExists(db, "SELECT id FROM challenges WHERE id = ?", battleID)

	if !isOpen {
		http.Redirect(w, r, redirectURL, 301)
		return
	}

	track := policy.Sanitize(r.FormValue("track"))

	if !strings.Contains(track, "soundcloud") {
		http.Redirect(w, r, "/beat/"+strconv.Itoa(battleID)+"/submit/sconly", 301)
		return
	}

	stmt := "INSERT INTO beats(url, challenge_id, user_id) VALUES(?,?,?)"
	response := "/successadd"

	// If has entered, update instead.
	if RowExists(db, "SELECT challenge_id FROM beats WHERE user_id = ? AND challenge_id = ?", user.ID, battleID) {
		stmt = "UPDATE beats SET url=? WHERE challenge_id=? AND user_id=?"
		response = "/successupdate"

	}

	ins, err := db.Prepare(stmt)
	if err != nil {
		panic(err.Error())
	}
	defer ins.Close()

	ins.Exec(track, battleID, user.ID)
	println(redirectURL + response)
	http.Redirect(w, r, redirectURL+response, 301)
	return
}

// DeleteBeat ...
func DeleteBeat(w http.ResponseWriter, r *http.Request) {
	db := dbConn()
	defer db.Close()

	user := GetUser(w, r)
	if !user.Authenticated {
		http.Redirect(w, r, "/login/noauth", 301)
		return
	}

	battleID, err := strconv.Atoi(r.URL.Query().Get(":id"))
	if err != nil {
		http.Redirect(w, r, "/404", 301)
		return
	}

	redirectURL := "/battle/" + strconv.Itoa(battleID)

	stmt := "DELETE FROM beats WHERE user_id = ? AND challenge_id = ?"
	ins, err := db.Prepare(stmt)
	if err != nil {
		http.Redirect(w, r, redirectURL+"/validationerror", 301)
	}
	defer ins.Close()

	ins.Exec(user.ID, battleID)

	http.Redirect(w, r, redirectURL+"/successdel", 301)
	return
}
